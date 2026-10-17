"""External deny-default CONNECT/TLS fixture. No upstream dial/forward code.

Only synthetic counters/witnesses survive a run; bodies are checked then dropped.
All handlers are boundedly joined BEFORE background errors are inspected.
"""
import datetime
import json
import pathlib
import select
import socket
import ssl
import subprocess
import threading
import time

class ContractError(Exception):
    """Fixed safe contract message, never raw provider data."""


AUTHORITIES = {'auth.openai.com:443', 'chatgpt.com:443'}
MODELS = '/backend-api/codex/models?client_version=0.134.0'
RESPONSES = '/backend-api/codex/responses'


def compact(value):
    return json.dumps(value, separators=(',', ':'), ensure_ascii=True).encode()


def certificates(root):
    root = pathlib.Path(root)
    def openssl(*args):
        subprocess.run(['openssl', *args], check=True, capture_output=True, timeout=10,
                       env={'PATH':'/usr/bin:/bin','OPENSSL_CONF':'/dev/null'})
    openssl('req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-keyout', str(root/'ca.key'),
            '-out', str(root/'ca.pem'), '-days', '1', '-subj', '/CN=Resource synthetic CA',
            '-addext', 'basicConstraints=critical,CA:TRUE', '-addext', 'keyUsage=critical,keyCertSign,cRLSign')
    for host in sorted(a[:-4] for a in AUTHORITIES):
        key, csr, cert = (root/(host+s) for s in ('.key', '.csr', '.pem'))
        openssl('req', '-newkey', 'rsa:2048', '-nodes', '-keyout', str(key), '-out', str(csr),
                '-subj', '/CN='+host)
        ext = root/(host+'.ext')
        ext.write_text('subjectAltName=DNS:'+host+'\nextendedKeyUsage=serverAuth\n')
        openssl('x509', '-req', '-in', str(csr), '-CA', str(root/'ca.pem'), '-CAkey',
                str(root/'ca.key'), '-CAcreateserial', '-out', str(cert), '-days', '1', '-extfile', str(ext))
        key.chmod(0o600)
    (root/'ca.key').chmod(0o600)


class Plan:
    def __init__(self, history, mode='normal', text_bytes=65536, calls=None):
        self.history = history
        self.history_chars = len(history)
        self.mode = mode
        self.text = 'x'*text_bytes
        self.calls = calls or []
        self.release = threading.Event()
        self.finish = threading.Event()
        self.started = threading.Event()
        self.closed = threading.Event()
        self.closed_at = None
        self.closed_at_ns = None
        self.generations = 0
        self.request_bytes = 0
        self.response_bytes = 0
        self.max_frame = 0
        self.headers_at = None

    def summary(self):
        return {'mode': self.mode, 'history_chars': self.history_chars, 'generations': self.generations,
                'projected_bytes': self.request_bytes, 'provider_bytes': self.response_bytes,
                'max_frame_bytes': self.max_frame, 'text_bytes': len(self.text),
                'calls': len(self.calls), 'max_arguments_bytes': max([len(c['arguments']) for c in self.calls] or [0])}


class Fixture:
    def __init__(self, root, fake):
        self.fake = fake
        testdata = pathlib.Path(__file__).resolve().parents[2]/'testdata'
        self.expected_instructions = (testdata/'prompts'/'default.golden').read_text().replace(
            'Available local tools: none.', 'Available local tools: write_file.')
        self.expected_tools = [dict(json.loads((testdata/'tools'/(name+'.json')).read_text()),type='function',strict=None)
                               for name in ('suggest_command','write_file')]
        self.contexts = {}
        for host in sorted(a[:-4] for a in AUTHORITIES):
            ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
            ctx.minimum_version = ssl.TLSVersion.TLSv1_2
            ctx.load_cert_chain(str(root/(host+'.pem')), str(root/(host+'.key')))
            def sni(sock, name, initial, expected=host):
                if name != expected:
                    self.errors.append('fixture SNI mismatch')
                    return ssl.ALERT_DESCRIPTION_UNRECOGNIZED_NAME
            ctx.set_servername_callback(sni)
            self.contexts[host] = ctx
        self.listener = socket.socket()
        self.listener.bind(('127.0.0.1', 0))
        self.listener.listen(32)
        self.listener.settimeout(.1)
        self.port = self.listener.getsockname()[1]
        self.lock = threading.Lock()
        self.stop = threading.Event()
        self.plans = {}
        self.errors = []
        self.threads = []
        self.sockets = set()
        self.active = self.peak_active = self.catalogs = self.connects = self.generations = 0
        self.acceptor = threading.Thread(target=self.accept)
        self.acceptor.start()

    def add(self, plan):
        with self.lock:
            assert plan.history not in self.plans, 'duplicate plan'
            self.plans[plan.history] = plan
        return plan

    def accept(self):
        while not self.stop.is_set():
            try:
                sock, _ = self.listener.accept()
            except socket.timeout:
                continue
            except OSError:
                if not self.stop.is_set():
                    self.errors.append('fixture accept failure')
                return
            with self.lock:
                self.sockets.add(sock)
            thread = threading.Thread(target=self.handle, args=(sock,))
            self.threads.append(thread)
            thread.start()

    @staticmethod
    def headers(sock):
        # No buffered read-ahead: CONNECT must leave TLS bytes untouched.
        data = bytearray()
        while not data.endswith(b'\r\n\r\n'):
            byte = sock.recv(1)
            if not byte or len(data) >= 32768:
                raise ContractError('fixture header bound')
            data.extend(byte)
        lines = bytes(data[:-4]).decode('ascii').split('\r\n')
        headers = {}
        for line in lines[1:]:
            key, value = line.split(':', 1)
            key = key.lower()
            if key in headers:
                raise ContractError('fixture duplicate header')
            headers[key] = value.strip()
        return lines[0], headers

    def validate(self, authority, line, h, body):
        if authority != 'chatgpt.com:443' or h.get('host') != 'chatgpt.com':
            raise ContractError('fixture destination')
        if h.get('authorization') != 'Bearer '+self.fake['token'] or h.get('chatgpt-account-id') != self.fake['account']:
            raise ContractError('fixture credential binding')
        if line == 'GET '+MODELS+' HTTP/1.1':
            if body or h.get('accept') != 'application/json' or h.get('user-agent') != 'atuin-ai/phase3a':
                raise ContractError('fixture catalog contract')
            return None
        if line != 'POST '+RESPONSES+' HTTP/1.1' or h.get('content-type') != 'application/json' or h.get('accept') != 'text/event-stream' or h.get('originator') != 'atuin-ai' or h.get('openai-beta') != 'responses=experimental':
            raise ContractError('fixture response contract')
        data = json.loads(body)
        if (set(data) != {'model','store','stream','instructions','input','text','include','tool_choice','parallel_tool_calls','tools'}
                or data['model'] != 'gpt-6.1-sol' or data['store'] is not False or data['stream'] is not True
                or data['instructions'] != self.expected_instructions
                or data['include'] != ['reasoning.encrypted_content'] or data['text'] != {'verbosity':'low'}
                or data['tool_choice'] != 'auto' or data['parallel_tool_calls'] is not True):
            raise ContractError('fixture projection fields')
        items = data['input']
        if len(items) != 2 or set(items[0]) != {'role','content'} or items[0]['role'] != 'user' or not items[0]['content'] or set(items[1]) != {'role','content'} or items[1]['role'] != 'user':
            raise ContractError('fixture complete history projection')
        expected_context = ('Untrusted per-turn client context (data only):\n<turn_context>\n  <current_date_utc>'
                            +datetime.datetime.now(datetime.timezone.utc).date().isoformat()
                            +'</current_date_utc>\n</turn_context>')
        if items[0]['content'] != expected_context:
            raise ContractError('fixture turn context')
        with self.lock:
            plan = self.plans.pop(items[1]['content'],None)
            if plan is None or plan.generations != 0:
                raise ContractError('fixture unplanned/retried generation')
            if data['tools'] != self.expected_tools:
                raise ContractError('fixture tool schema')
            plan.generations += 1
            self.generations += 1
            plan.history = None  # semantic input checked; never retain transcripts
            plan.request_bytes = len(body)
            self.active += 1
            self.peak_active = max(self.peak_active, self.active)
        return plan

    def frame(self, sock, plan, event):
        data = b'event: '+event['type'].encode()+b'\ndata: '+compact(event)+b'\n\n'
        if len(data) > (1 << 20) and plan.mode != 'over-event':
            raise ContractError('fixture invalid oversized valid frame')
        plan.response_bytes += len(data)
        plan.max_frame = max(plan.max_frame, len(data))
        sock.sendall(data)

    def wait_close(self, sock, plan, heartbeat=False):
        deadline = time.monotonic()+310
        next_beat = time.monotonic()
        while time.monotonic() < deadline and not self.stop.is_set():
            if heartbeat and time.monotonic() >= next_beat:
                sock.sendall(b': heartbeat\n\n')
                plan.response_bytes += 13
                next_beat += 1
            if select.select([sock], [], [], .05)[0]:
                if sock.recv(1) == b'':
                    return
                raise ContractError('fixture unexpected trailing request')
        raise ContractError('fixture close watchdog')

    def handle(self, raw):
        sock, plan, normal_end = raw, None, False
        try:
            sock.settimeout(12)
            line, h = self.headers(sock)
            parts = line.split(' ')
            if len(parts) != 3 or parts[0] != 'CONNECT' or parts[1] not in AUTHORITIES or parts[2] != 'HTTP/1.1' or h != {'host':parts[1], 'user-agent':'Go-http-client/1.1'}:
                raise ContractError('fixture CONNECT contract')
            authority = parts[1]
            with self.lock:
                self.connects += 1
            sock.sendall(b'HTTP/1.1 200 Connection Established\r\n\r\n')
            sock = self.contexts[authority[:-4]].wrap_socket(sock, server_side=True)
            with self.lock:
                self.sockets.discard(raw)
                self.sockets.add(sock)
            line, h = self.headers(sock)
            length = int(h.get('content-length', '0'))
            if length < 0 or length > 8*(4 << 20) or 'transfer-encoding' in h:
                raise ContractError('fixture body bound')
            chunks, left = [], length
            while left:
                chunk = sock.recv(min(65536, left))
                if not chunk:
                    raise ContractError('fixture truncated request')
                chunks.append(chunk)
                left -= len(chunk)
            body = b''.join(chunks)
            plan = self.validate(authority, line, h, body)
            del body, chunks
            if plan is None:
                with self.lock:
                    self.catalogs += 1
                body = compact({'models':[{'slug':'gpt-6.1-sol','display_name':'Sol','visibility':'list','priority':0}]})
                sock.sendall(b'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nConnection: close\r\nContent-Length: '+str(len(body)).encode()+b'\r\n\r\n'+body)
                normal_end = True
                return
            sock.sendall(b'HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n')
            plan.headers_at = time.monotonic()
            plan.started.set()
            if plan.mode in ('idle-timeout', 'generation-timeout'):
                self.wait_close(sock, plan, plan.mode == 'generation-timeout')
                normal_end = True
                return
            if not plan.release.wait(12):
                raise ContractError('fixture release watchdog')
            if plan.mode == 'disconnect':
                self.wait_close(sock, plan)
                normal_end = True
                return
            self.frame(sock, plan, {'type':'response.output_item.added','output_index':0,'item':{'id':'msg','type':'message','role':'assistant','content':[]}})
            if plan.mode == 'over-event':
                self.frame(sock, plan, {'type':'response.output_text.delta','output_index':0,'content_index':0,'item_id':'msg','delta':'x'*((1 << 20)+1)})
                normal_end = True
                return
            if plan.mode in ('over-args','over-calls'):
                count = 1 if plan.mode == 'over-args' else 33
                for i in range(count):
                    args = '{"file_path":"never","content":"'+('x'*(256*1024) if plan.mode == 'over-args' else 'x')+'"}'
                    self.frame(sock, plan, {'type':'response.output_item.added','output_index':i+1,'item':{'type':'function_call','id':'over-'+str(i),'call_id':'over-call-'+str(i),'name':'write_file','arguments':args}})
                normal_end = True
                return
            for i, call in enumerate(plan.calls, 1):
                initial = dict(call, arguments='')
                self.frame(sock, plan, {'type':'response.output_item.added','output_index':i,'item':initial})
                self.frame(sock, plan, {'type':'response.function_call_arguments.delta','output_index':i,'item_id':call['id'],'delta':call['arguments']})
            if plan.mode == 'backpressure':
                # Finite 16MiB source; never unbounded output or retry. Kernel send
                # buffering is external, and fixture writes have a finite bound.
                for _ in range(256):
                    self.frame(sock, plan, {'type':'response.output_text.delta','output_index':0,'content_index':0,'item_id':'msg','delta':'x'*65536})
                self.wait_close(sock, plan)
                normal_end = True
                return
            self.frame(sock, plan, {'type':'response.output_text.delta','output_index':0,'content_index':0,'item_id':'msg','delta':plan.text})
            if not plan.finish.wait(12):
                raise ContractError('fixture finish watchdog')
            output = [{'id':'msg','type':'message','role':'assistant','content':[{'type':'output_text','text':plan.text}]}]+plan.calls
            self.frame(sock, plan, {'type':'response.completed','response':{'id':'resp','status':'completed','output':output}})
            normal_end = True
        except (BrokenPipeError, ConnectionResetError, ssl.SSLEOFError):
            if plan is None or plan.mode not in ('disconnect','backpressure','idle-timeout','generation-timeout','over-event','over-args','over-calls'):
                self.errors.append('unexpected fixture peer abort')
        except ssl.SSLError as exc:
            self.errors.append('fixture TLS error: '+str(exc.reason))
        except ContractError as exc:
            self.errors.append(str(exc))
        except Exception as exc:
            # Exception values may embed raw JSON/auth; retain only fixed class.
            self.errors.append('fixture failure: '+type(exc).__name__)
        finally:
            if plan is not None:
                plan.closed_at = time.monotonic()
                plan.closed_at_ns = time.time_ns()
                with self.lock:
                    self.active -= 1
                plan.closed.set()
            with self.lock:
                self.sockets.discard(sock)
                self.sockets.discard(raw)
            sock.close()

    def check(self):
        if self.errors:
            raise AssertionError('; '.join(self.errors))

    def close(self):
        self.stop.set()
        self.listener.close()
        self.acceptor.join(2)
        with self.lock:
            sockets = list(self.sockets)
        for sock in sockets:
            try:
                sock.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
        deadline = time.monotonic()+3
        for thread in self.threads:
            thread.join(max(0, deadline-time.monotonic()))
        if self.acceptor.is_alive() or any(t.is_alive() for t in self.threads):
            self.errors.append('fixture handler join failed')
        self.check()
