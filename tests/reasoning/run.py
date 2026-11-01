#!/usr/bin/env python3
"""Separate offline reasoning CLI/restart and storage-resource proof.

No backend knob, live auth, host service, broad checkout mount or external dial.
Actual native destinations are reached only through a deny-default synthetic
CONNECT/TLS fixture in an isolated network/PID/user namespace.
"""
import argparse
import base64
import concurrent.futures
import hashlib
import http.client
import http.server
import json
import os
import pathlib
import pty
import select
import shutil
import signal
import socket
import ssl
import stat
import subprocess
import sys
import tempfile
import threading
import time

HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))
sys.path.insert(0, str(HERE.parent/'resources'))
from bwrap_tool import resolve
from tls_fixture import Fixture, Plan, certificates, compact, RESPONSES

INPUTS = ('tests/reasoning/run.py', 'tests/e2e/pty_driver.py',
          'tests/resources/tls_fixture.py', 'tests/bwrap_tool.py',
          'tests/build_support.py', 'testdata/prompts/default.golden',
          'testdata/tools/suggest_command.json', 'testdata/tools/write_file.json')
MIB = 1 << 20
PRIVATE = 'SYNTHETIC_REASONING_PRIVATE_2817'


def require(value, message):
    if not value:
        raise AssertionError(message)


def digest(path):
    return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()


def frame(event):
    return b'event: '+event['type'].encode()+b'\ndata: '+compact(event)+b'\n\n'


class ReasoningPlan(Plan):
    def __init__(self, name, text, calls=(), reasoning=True, missing=False):
        super().__init__(name, text_bytes=0, calls=list(calls))
        self.name, self.text = name, text
        self.reason = ({'type':'reasoning','id':'reason-'+name,
                        'summary':[{'type':'summary_text','text':PRIVATE+'-summary'}],
                        'encrypted_content':PRIVATE+'-encrypted-'+name,
                        'status':'completed'} if reasoning else None)
        if missing:
            del self.reason['encrypted_content']
        self.release.set()
        self.finish.set()
        self.request = None
        self.expected = None

    def output(self):
        rows = [{'type':'message','id':'msg-'+self.name,'role':'assistant','status':'completed','phase':'commentary',
                 'content':[{'type':'output_text','text':self.text,'annotations':[]}]}]+self.calls
        if self.reason is not None:
            rows.append(self.reason)
        return rows

    def replay_output(self):
        rows=json.loads(json.dumps(self.output()))
        for row in rows:
            if row['type']=='function_call':
                for field in ('async','namespace','status'):
                    if field in row and row[field] is None:
                        del row[field]
            if row['type']=='message':
                for part in row['content']:
                    if part.get('logprobs','absent') is None:
                        del part['logprobs']
        return rows


def validate_replay_schema(items):
    for row in items:
        if row.get('type')=='message':
            require(all(k in row for k in ('type','id','role','status','content')) and row['role']=='assistant' and row['status']=='completed','required schema-valid output message replay')
            require('phase' not in row or row['phase'] in (None,'commentary','final_answer'),'message phase schema')
            require(isinstance(row['content'],list),'message content array')
            for part in row['content']:
                require(part.get('type')=='output_text' and isinstance(part.get('text'),str) and part.get('annotations')==[],'required supported empty annotations')
                require('logprobs' not in part or part['logprobs']==[],'nonnull supported replay logprobs')
        if row.get('type')=='function_call':
            require(all(isinstance(row.get(k),str) for k in ('arguments','call_id','name')),'required function replay fields')
            require('caller' not in row or row['caller'] is None or row['caller']=={'type':'direct'},'supported direct caller replay')
            require('async' not in row or row['async'] is False,'supported nonnull synchronous replay')


class ReasoningFixture(Fixture):
    def __init__(self, root, fake):
        self.queue = []
        self.captured = []
        super().__init__(root, fake)

    def add_reasoning(self, plan):
        with self.lock:
            self.queue.append(plan)
        return plan

    def validate(self, authority, line, headers, body):
        require(authority == 'chatgpt.com:443' and headers.get('host') == 'chatgpt.com', 'fixed native authority')
        require(line == 'POST '+RESPONSES+' HTTP/1.1', 'no catalog/auth/fallback destination')
        require(headers.get('authorization') == 'Bearer '+self.fake['token']
                and headers.get('chatgpt-account-id') == self.fake['account'], 'synthetic credential binding')
        require(headers.get('accept') == 'text/event-stream' and headers.get('content-type') == 'application/json'
                and headers.get('openai-beta') == 'responses=experimental' and headers.get('originator') == 'atuin-ai', 'native wire headers')
        data = json.loads(body)
        require(set(data) == {'model','store','stream','instructions','input','text','include','tool_choice','parallel_tool_calls','tools'}
                and data['model'] == 'gpt-6.1-sol' and data['store'] is False and data['stream'] is True
                and data['include'] == ['reasoning.encrypted_content'] and data['text'] == {'verbosity':'low'}
                and data['tool_choice'] == 'auto' and data['parallel_tool_calls'] is True, 'native request envelope')
        validate_replay_schema(data['input'])
        with self.lock:
            require(bool(self.queue), 'unplanned native generation/retry')
            # CLI is serialized; parallel stress plans have distinguishable
            # original user history markers, never inferred from protected IDs.
            choices = [p for p in self.queue if any(p.history in row.get('content','') for row in data['input'] if isinstance(row.get('content'),str))]
            plan = choices[0] if choices else self.queue[0]
            self.queue.remove(plan)
            require(plan.generations == 0, 'retry forbidden')
            if plan.expected is not None:
                require(data['input'][1:] == plan.expected, 'exact full provider output/results/order replay')
            require(data['input'][0]['role'] == 'user' and '<turn_context>' in data['input'][0]['content'], 'volatile turn context')
            plan.request = data
            plan.request_bytes = len(body)
            plan.generations += 1
            self.generations += 1
            self.active += 1
            self.peak_active = max(self.peak_active,self.active)
            self.captured.append({'plan':plan.name,'bytes':len(body),'sha256':hashlib.sha256(body).hexdigest()})
        return plan

    def frame(self, sock, plan, event):
        # Reuse original external fixture framing/bounds/body close observation,
        # but assign globally distinct retained item IDs and final-only reasoning.
        if plan.mode in ('checkpoint-memory','ordinary-checkpoint-memory'):
            if event['type']=='response.output_item.added':
                payload=PRIVATE+'z'*(900*1024)
                for index in range(128):
                    if plan.mode=='checkpoint-memory':
                        item={'type':'reasoning','id':plan.name+'-'+str(index),'summary':[],'encrypted_content':payload,'status':'completed'}
                    else:
                        item={'type':'message','id':plan.name+'-'+str(index),'role':'assistant','status':'completed','content':[],
                              'name':'n'*(300*1024),'call_id':'c'*(300*1024),'arguments':'a'*(256*1024)}
                    checkpoint={'type':'response.output_item.done','sequence_number':index,'output_index':index,'item':item}
                    Fixture.frame(self,sock,plan,checkpoint)
            return # EOF without completed; no raw DONE snapshots may accumulate.
        event = json.loads(json.dumps(event))
        if event.get('item_id') == 'msg':
            event['item_id'] = 'msg-'+plan.name
        if event.get('item',{}).get('id') == 'msg':
            event['item']['id'] = 'msg-'+plan.name
        if event['type'] == 'response.completed':
            event['response']['id'] = 'response-'+plan.name
            event['response']['output'] = plan.output()
        super().frame(sock,plan,event)


class Server:
    def __init__(self, env, work, executable='/binary', companion=False):
        self.env,self.work,self.executable,self.companion = env,work,executable,companion
        self.process = None
        self.logs = []
        self.pids = []
        self.max_rss = 0
        self.max_fds = 0
        self.stop_sample = threading.Event()
        self.samples = []
        self.sample_lock=threading.Lock()
        self.sample_errors=[]
        self.coverage={}
        self.thread = threading.Thread(target=self.sample)
        self.thread.start()

    def read_measurement(self,pid):
        status=pathlib.Path('/proc',str(pid),'status').read_text()
        values={row.split(':')[0]:int(row.split()[1])*1024 for row in status.splitlines() if row.startswith(('VmRSS:','VmHWM:'))}
        require(set(values)=={'VmRSS','VmHWM'} and all(n>0 for n in values.values()),'complete positive owned-process memory sample required')
        fds=len(list(pathlib.Path('/proc',str(pid),'fd').iterdir()))
        return max(values.values()),fds

    def observe_locked(self,phase):
        process=self.process
        if process is None:
            return
        row=self.coverage.setdefault(process.pid,{'samples':0,'baseline':False,'final':False})
        try:
            require(process.poll() is None,'owned server exited during measurement')
            rss,fds=self.read_measurement(process.pid)
            self.max_rss,self.max_fds=max(self.max_rss,rss),max(self.max_fds,fds)
            self.samples.append((time.monotonic(),rss,fds));row['samples']+=1
            if phase in ('baseline','final'):
                row[phase]=True
        except BaseException as exc:
            self.sample_errors.append('owned process sampling failed: '+type(exc).__name__)

    def mark_baseline(self):
        with self.sample_lock:
            self.observe_locked('baseline')
        require(not self.sample_errors,'; '.join(self.sample_errors))

    def sample(self):
        try:
            while not self.stop_sample.wait(.02):
                with self.sample_lock:
                    self.observe_locked('periodic')
        except BaseException as exc:
            self.sample_errors.append('sampler thread failed: '+type(exc).__name__)

    def check_measurements(self):
        require(not self.sample_errors,'; '.join(self.sample_errors))
        require(bool(self.coverage) and bool(self.samples),'nonzero sampler coverage required')
        require(all(row['samples']>=2 and row['baseline'] and row['final'] for row in self.coverage.values()),'mandatory before/after-workload memory samples required')

    def start(self):
        require(self.process is None,'owned server already running')
        log = self.work/(('companion-' if self.companion else 'server-')+str(len(self.pids))+'.log')
        self.logs.append(log)
        stream = log.open('wb')
        args = [self.executable,'serve'] if self.companion else [self.executable,'serve','--config',str(self.work/'server.toml')]
        try:
            self.process = subprocess.Popen(args,stdin=subprocess.PIPE if self.companion else subprocess.DEVNULL,
                                            stdout=subprocess.PIPE if self.companion else stream,stderr=stream,
                                            env=self.env,start_new_session=True)
        finally:
            stream.close()
        self.pids.append(self.process.pid)
        if self.companion:
            require(json.loads(self.process.stdout.readline()) == {'companion_ready':True},'companion readiness')
        deadline = time.monotonic()+8
        while time.monotonic()<deadline:
            require(self.process.poll() is None,'server startup failed')
            try:
                conn=http.client.HTTPConnection('127.0.0.1',17391,timeout=.2)
                conn.request('GET','/api/cli/models');response=conn.getresponse();response.read();conn.close()
                if response.status == 200:
                    self.mark_baseline()
                    return
            except (OSError,http.client.HTTPException):
                time.sleep(.02)
        raise AssertionError('owned server readiness deadline')

    def snapshot(self):
        require(self.companion,'GC snapshots are companion-only, never shipping RSS')
        self.process.stdin.write(b'snapshot\n');self.process.stdin.flush()
        return json.loads(self.process.stdout.readline())

    def stop(self):
        with self.sample_lock:
            process=self.process
            if process is not None:
                self.observe_locked('final')
            self.process=None
        if process is None:
            return
        process.send_signal(signal.SIGTERM)
        try:
            require(process.wait(timeout=8) == 0,'server signal shutdown failed')
        except BaseException:
            process.kill();process.wait(timeout=3)
            raise
        if process.stdin is not None:
            process.stdin.close()
        if process.stdout is not None:
            process.stdout.close()

    def close(self):
        try:
            self.stop()
        finally:
            self.stop_sample.set();self.thread.join(2)
        require(not self.thread.is_alive(),'sampler did not terminate after bounded join')
        self.check_measurements()
        require(self.max_rss<=256*MIB,'unchanged shipping active RSS hard budget')
        for path in self.logs:
            require(PRIVATE.encode() not in path.read_bytes(),'private reasoning in production logs')


def wire_request(body,port=17391):
    conn=http.client.HTTPConnection('127.0.0.1',port,timeout=15)
    conn.request('POST','/api/cli/chat',body,{'Content-Type':'application/json'})
    response=conn.getresponse();data=response.read();session=response.getheader('X-Atuin-AI-Session-Id');conn.close()
    require(response.status==200,'actual chat HTTP status '+str(response.status))
    require(PRIVATE.encode() not in data,'private reasoning in public SSE')
    events=[]
    for packet in data.split(b'\n\n'):
        rows=packet.splitlines()
        if len(rows)>=2 and rows[0].startswith(b'event: ') and rows[1].startswith(b'data: '):
            events.append((rows[0][7:].decode(),json.loads(rows[1][6:])))
    return events,session


def calls(name):
    return [{'type':'function_call','id':'item-'+name,'call_id':'backend-'+name,
             'name':'read_file','status':'completed','caller':{'type':'direct'},'async':False,'arguments':'{ "file_path":"fixture.txt", "offset":0,"limit":10,"extra":{"unsigned":18446744073709551615,"signed":-9223372036854775808} }'}]


def public_history(body,events,session,text=None):
    request=json.loads(body)
    request['session_id']=session
    tool_calls=[v for name,v in events if name=='tool_call']
    require(tool_calls and all(len(c['id'])==90 and c['id'].startswith('aic1_') for c in tool_calls),'protected public handles')
    if text is None:
        text=''.join(v['content'] for name,v in events if name=='text').lstrip()
    blocks=([{'type':'text','text':text}] if text else [])+[dict(c,type='tool_use') for c in tool_calls]
    request['messages'].append({'role':'assistant','content':blocks})
    request['messages'].append({'role':'user','content':[{'type':'tool_result','tool_use_id':c['id'],'content':'OWNED_RESULT_'+str(j)} for j,c in enumerate(tool_calls)]})
    return compact(request),tool_calls


def mapped_results(request,original_calls,public_ids):
    mapping=dict(zip(public_ids,[c['call_id'] for c in original_calls]));out=[]
    for message in request['messages']:
        content=message['content']
        if not isinstance(content,list):
            continue
        for block in content:
            if block.get('type')=='tool_result' and block.get('tool_use_id') in mapping:
                result=block['content']
                if isinstance(result,list):
                    result='\n'.join(x.get('text','') for x in result if x.get('type')=='text')
                if block.get('is_error'):
                    result='Tool execution failed: '+result
                out.append({'type':'function_call_output','call_id':mapping[block['tool_use_id']],'output':result})
    require(len(out)==len(original_calls),'whole-K actual result correlation')
    return out


class Proxy(http.server.ThreadingHTTPServer):
    daemon_threads=False
    def __init__(self,server,fixture,plans,credentials):
        self.server,self.fixture,self.plans,self.credentials=server,fixture,plans,credentials
        self.requests=[];self.ids=[];self.sessions=[];self.errors=[];self.restarts=0
        super().__init__(('127.0.0.1',9997),ProxyHandler)


class ProxyHandler(http.server.BaseHTTPRequestHandler):
    protocol_version='HTTP/1.0'
    def log_message(self,*args):
        pass
    def do_GET(self):
        if self.path=='/__release':
            # Existing PTY driver release action advances its output checkpoint.
            # No native plan is released/retried by this synchronization hook.
            self.send_response(204);self.send_header('Content-Length','0');self.end_headers();return
        conn=http.client.HTTPConnection('127.0.0.1',17391,timeout=10);conn.request('GET',self.path);r=conn.getresponse();data=r.read();self.send_response(r.status);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(data);conn.close()
    def do_POST(self):
        owner=self.server
        try:
            require(self.path=='/api/cli/chat','closed proxy routes')
            length=int(self.headers.get('Content-Length','0'));require(0<length<=4*MIB,'CLI body bounds')
            body=self.rfile.read(length);request=json.loads(body);index=len(owner.requests);owner.requests.append(request)
            require(index<len(owner.plans),'no CLI retry or fallback')
            if index:
                require(request.get('session_id')==owner.sessions[0],'actual CLI lost server session')
                base=owner.plans[0].request['input'][1:];expected=list(base)
                for prior in range(index):
                    expected+=owner.plans[prior].replay_output()
                    expected+=mapped_results(request,owner.plans[prior].calls,owner.ids[prior])
                owner.plans[index].expected=expected
            owner.fixture.add_reasoning(owner.plans[index])
            conn=http.client.HTTPConnection('127.0.0.1',17391,timeout=15);conn.request('POST',self.path,body,{'Content-Type':'application/json'});r=conn.getresponse();require(r.status==200,'actual CLI server request')
            session=r.getheader('X-Atuin-AI-Session-Id');owner.sessions.append(session)
            self.send_response(200);self.send_header('Content-Type','text/event-stream');self.send_header('X-Atuin-AI-Session-Id',session);self.end_headers();packet=bytearray();ids=[]
            while True:
                row=r.readline()
                if not row:
                    break
                packet.extend(row)
                if row==b'\n':
                    require(PRIVATE.encode() not in packet,'private reasoning escaped SSE proxy')
                    if packet.startswith(b'event: tool_call\n'):
                        c=json.loads(bytes(packet).split(b'data: ',1)[1]);require(c['id'].startswith('aic1_') and len(c['id'])==90,'real CLI never received protected ID');ids.append(c['id'])
                    if packet.startswith(b'event: done\n'):
                        owner.ids.append(ids)
                        if index<2:
                            # Hold ONLY done, not tool/status/text. This proves
                            # an actual server process restart before the CLI's
                            # automatic continuation can start, without a retry.
                            before=digest(owner.credentials);owner.server.stop();owner.server.start();require(digest(owner.credentials)==before,'ordinary process restart rewrote credential JSON');owner.restarts+=1
                    self.wfile.write(packet);self.wfile.flush();packet.clear()
            conn.close()
        except BaseException as exc:
            owner.errors.append(type(exc).__name__+': '+str(exc) if isinstance(exc,AssertionError) else type(exc).__name__)
            self.close_connection=True


def cli_proof(server,fixture,work,credentials):
    skill=lambda name:[{'type':'function_call','id':'item-'+name,'call_id':'backend-'+name,'name':'load_skill','status':'completed','caller':None,'async':None,'arguments':'{ "name":"synthetic", "extra":{"unsigned":18446744073709551615,"signed":-9223372036854775808} }'}]
    plans=[ReasoningPlan('cli-0','\u2003 \tREASON_PHASE_ONE \n',calls('cli-a')+calls('cli-b')),
           ReasoningPlan('cli-1','\tREASON_PHASE_TWO \n',skill('cli-c'),reasoning=False),
           ReasoningPlan('cli-2','REASONING_COMPLETE',reasoning=True)]
    proxy=Proxy(server,fixture,plans,credentials);thread=threading.Thread(target=proxy.serve_forever);thread.start()
    try:
        plan={'binary':'/atuin','root':'/fixture','endpoint':'http://127.0.0.1:9997','prompt':'REASON_CHAIN',
              'steps':[{'wait':'Always allow in this directory'},{'send':'\u001b[B'},{'send':'\r','release':True},
                       {'wait':'(?s)(?:✓.*Allow|Allow.*✓)','fresh':True},{'send':'\r'},
                       {'wait':'(?s)REASONING_COMPLETE.*Responded in'},{'send':'\u001b'}]}
        completed=subprocess.run(['/usr/bin/python3','-B','-I','/repo/tests/e2e/pty_driver.py'],input=compact(plan),capture_output=True,timeout=60,env=dict(os.environ,ATUIN_E2E_NAMESPACE='1'))
        require(not proxy.errors,'; '.join(proxy.errors))
        require(completed.returncode==0,'pinned CLI private-PTY reasoning scenario: '+completed.stderr.decode(errors='replace')[-2000:])
        require(len(proxy.requests)==3 and len(proxy.ids)==3 and proxy.restarts==2,'actual CLI/restart/generation count')
        require([len(x) for x in proxy.ids]==[2,1,0],'whole-K protected executable batches')
        require(fixture.generations==3 and fixture.catalogs==0,'one native generation, zero catalogs')
        all_ids=[v for row in proxy.ids for v in row];require(len(set(all_ids))==3,'distinct opaque IDs')
        for index in (1,2):
            request=proxy.requests[index]
            public_calls=[b for m in request['messages'] if isinstance(m['content'],list) for b in m['content'] if b.get('type')=='tool_use']
            require([c['id'] for c in public_calls]==[v for row in proxy.ids[:index] for v in row],'actual CLI altered/omitted protected history IDs')
            for c in public_calls:
                require(c['input']['extra']=={'unsigned':18446744073709551615,'signed':-9223372036854775808},'actual Rust integer JSON loss')
        return {'case':'actual-pinned-cli-reasoning-restart','generations':3,'catalogs':0,'server_processes':len(server.pids),'restarts_between_tool_batches':2,'batch_sizes':[2,1,0],'exact_original_output_results':True,'raw_args_uint64_int64':True,'leading_only_trim_trailing_preserved':True,'session':proxy.sessions[0],'pids':server.pids,'native_requests':fixture.captured}
    finally:
        pathlib.Path('/evidence/cli-progress.json').write_bytes(json.dumps({'client_requests':len(proxy.requests),'tool_batches':[len(x) for x in proxy.ids],'restarts':proxy.restarts,'errors':proxy.errors,'native_requests':fixture.captured},sort_keys=True,indent=2).encode()+b'\n')
        proxy.shutdown();proxy.server_close();thread.join(3)


def packaged_init_proof(work,base_env):
    def isolated(name):
        root=work/name
        return root,dict(base_env,HOME=str(root/'home'),XDG_CONFIG_HOME=str(root/'config'),TERM='xterm-256color')
    root,env=isolated('init-no-tty')
    result=subprocess.run(['/binary','init'],stdin=subprocess.DEVNULL,capture_output=True,env=env,timeout=8)
    require(result.returncode!=0 and not root.exists(),'packaged no-TTY init mutated state')
    pathlib.Path('/evidence/init-no-tty.log').write_bytes(result.stdout+result.stderr)
    root,env=isolated('init-private-tty');root.mkdir(mode=0o700)
    pid,fd=pty.fork()
    if pid==0:
        os.execve('/binary',['/binary','init'],env)
    raw=bytearray();sent=False;status=None;deadline=time.monotonic()+12
    try:
        while time.monotonic()<deadline:
            if select.select([fd],[],[],.1)[0]:
                try:
                    chunk=os.read(fd,4096)
                except OSError:
                    chunk=b''
                if chunk:
                    raw.extend(chunk);require(len(raw)<=65536,'init PTY transcript bound')
                    if not sent and b'Log in now? [y/N]' in raw:
                        os.write(fd,b'n\n');sent=True
            done,value=os.waitpid(pid,os.WNOHANG)
            if done:
                status=os.waitstatus_to_exitcode(value);break
        require(sent and status==0,'packaged controlling-PTY explicit login decline failed')
        state=root/'config'/'atuin-ai'/'credentials'
        require(not (state/'continuation-v1').exists() and not (state/'record.json').exists(),'packaged init refusal created credentials/private continuation')
        require(b'RESOURCE-FAKE' not in raw and PRIVATE.encode() not in raw,'init leaked fake/private state')
        return {'no_tty_no_mutation':True,'controlling_private_tty_explicit_login_decline':True,'no_credential_or_continuation_payload':True}
    finally:
        pathlib.Path('/evidence/init-private-tty.log').write_bytes(raw)
        os.close(fd)
        if status is None:
            try:
                os.killpg(pid,signal.SIGKILL)
            except ProcessLookupError:
                pass
            os.waitpid(pid,0)


def stress_body(marker,near=False):
    request={'messages':[{'role':'user','content':marker}], 'config':{'capabilities':['client_v1_write_file']}}
    body=compact(request)
    if near:
        start=body.index(b'"content":"')+len(b'"content":"');end=body.index(b'"}',start)
        room=4*MIB-1024-start-len(body[end:]);decoded=marker+'a'*((room-len(marker))//6)+'b'*((room-len(marker))%6)
        body=body[:start]+marker.encode()+b'\\u0061'*((room-len(marker))//6)+b'b'*((room-len(marker))%6)+body[end:]
        require(len(body)==4*MIB-1024 and json.loads(body)['messages'][0]['content']==decoded,'near exact wire/decoded history')
    return body


def stress_call(name,near=False):
    return [{'type':'function_call','id':'item-'+name,'call_id':'backend-'+name,'name':'write_file',
             'arguments':compact({'file_path':'never-executed-'+name,'content':('e\u0301'*20000 if near else 'z'*8)}).decode()}]


def run_stress_workload(server,fixture,label):
    start=fixture.generations;handled=[];fd_samples=[]
    def clean_sample():
        time.sleep(.15)
        return len(list(pathlib.Path('/proc',str(server.process.pid),'fd').iterdir()))
    baseline=clean_sample()
    for index in range(8):
        name=label+'-failed-'+str(index);plan=fixture.add_reasoning(ReasoningPlan(name,'failure ',stress_call(name),missing=True));events,_=wire_request(stress_body(name));require([n for n,_ in events].count('error')==1 and not any(n in ('tool_call','done') for n,_ in events),'missing reasoning state released callable batch');require(plan.generations==1,'failed generation retried')
    for index in range(8):
        name=label+'-handled-'+str(index);plan=fixture.add_reasoning(ReasoningPlan(name,'handled ',stress_call(name)));body=stress_body(name);events,session=wire_request(body);require(any(n=='done' for n,_ in events),'handled generation failed');next_body,public=public_history(body,events,session);handled.append((plan,next_body,public));fd_samples.append(clean_sample())
    for index,(prior,body,public) in enumerate(handled):
        name=label+'-replayed-'+str(index);plan=ReasoningPlan(name,'replayed ',stress_call(name),reasoning=False);plan.expected=prior.request['input'][1:]+prior.replay_output()+mapped_results(json.loads(body),prior.calls,[c['id'] for c in public]);fixture.add_reasoning(plan);events,session=wire_request(body);require(any(n=='done' for n,_ in events),'inherited executable replay failed');public_history(body,events,session);fd_samples.append(clean_sample())
    for index in range(4):
        name=label+'-recovery-'+str(index);fixture.add_reasoning(ReasoningPlan(name,'recovered '));events,_=wire_request(stress_body(name));require(any(n=='done' for n,_ in events) and not any(n=='error' for n,_ in events),'post-cycle success recovery')
    require(server.max_rss<=64*MIB,'unchanged warmed-idle HWM hard budget before near load')
    normal_peak=server.max_rss
    near=[]
    for index in range(4):
        name=label+'-near-'+str(index);body=stress_body(name,True);plan=ReasoningPlan(name,' '*200000+'near ',stress_call(name,True));plan.reason['encrypted_content']=PRIVATE+'e'*600000;plan.release.clear();fixture.add_reasoning(plan);near.append((body,plan))
    def burst(plans):
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            futures=[pool.submit(wire_request,body) for body,plan in plans]
            for _,plan in plans:
                if not plan.started.wait(10):
                    for future in futures:
                        if future.done():
                            future.result()
                    raise AssertionError('four near scopes admitted')
            require(fixture.active==4,'four concurrent native scopes')
            for _,plan in plans:
                plan.release.set()
            return [future.result(timeout=90) for future in futures]
    first=burst(near);next_plans=[]
    for index,((body,prior),(events,session)) in enumerate(zip(near,first)):
        body,public=public_history(body,events,session);name=label+'-near-next-'+str(index);plan=ReasoningPlan(name,' '*200000+'near again ',stress_call(name,True));plan.reason['encrypted_content']=PRIVATE+'f'*600000;plan.history=prior.history;plan.expected=prior.request['input'][1:]+prior.replay_output()+mapped_results(json.loads(body),prior.calls,[c['id'] for c in public]);plan.release.clear();fixture.add_reasoning(plan);next_plans.append((body,plan))
    second=burst(next_plans)
    for (body,_),(events,session) in zip(next_plans,second):
        require(any(n=='done' for n,_ in events),'near-limit second protected segment failed');public_history(body,events,session)
    for mode in ('checkpoint-memory','ordinary-checkpoint-memory'):
        checkpoints=[]
        for index in range(4):
            name=label+'-'+mode+'-'+str(index);plan=ReasoningPlan(name,'',reasoning=False);plan.mode=mode;plan.release.clear();fixture.add_reasoning(plan);checkpoints.append((stress_body(name),plan))
        adversarial=burst(checkpoints)
        for events,_ in adversarial:
            require([name for name,_ in events].count('error')==1 and not any(name in ('tool_call','done') for name,_ in events),'128 large DONE checkpoints must terminate without tools on EOF')
    ending=clean_sample();require(ending<=baseline,'shipping descriptor recovery');require(fixture.generations-start==44,'separate stress exact generation count')
    return {'generations':44,'ordinary_adversarial_concurrent_scopes':4,'ordinary_done_checkpoints_per_scope':128,'ordinary_inapplicable_name_bytes':300*1024,'ordinary_inapplicable_call_id_bytes':300*1024,'ordinary_inapplicable_argument_bytes':256*1024,'adversarial_concurrent_scopes':4,'done_checkpoints_per_adversarial_scope':128,'encrypted_bytes_per_checkpoint':900*1024+len(PRIVATE),'failed':8,'handled':8,'replayed_executable':8,'post_cycle_success':4,'near_limit_concurrent_scopes':4,'near_generations':8,'fds_baseline':baseline,'fds_after':ending,'fds_samples':fd_samples,'normal_peak_rss_hwm_bytes':normal_peak}


def stress_proof(server,fixture,work):
    shipping=run_stress_workload(server,fixture,'shipping');server.stop()
    directory=work/'config'/'atuin-ai'/'credentials'/'continuation-v1';files=list(directory.iterdir());sizes=[p.stat().st_size for p in files];require(len(files)<=128+1 and sum(sizes)<=64*MIB and max(sizes)<=2*MIB,'same actual disk quota/record budget')
    companion=Server(server.env,work,executable='/helper',companion=True)
    summary=None
    try:
        companion.start()
        for index in range(4):
            name='warm-'+str(index);fixture.add_reasoning(ReasoningPlan(name,'warmed '));wire_request(stress_body(name))
        time.sleep(.2);before=companion.snapshot();result=run_stress_workload(companion,fixture,'companion');time.sleep(.2);after=companion.snapshot()
        allowances={'goroutines':2,'metric_goroutines':2,'heap_alloc':512*1024,'heap_objects':512}
        pathlib.Path('/evidence/companion-retention-observations.json').write_bytes(json.dumps({'before':before,'after':after,'allowances':allowances},sort_keys=True,indent=2).encode()+b'\n')
        for key,allowance in allowances.items():
            require(after[key]-before[key]<=allowance,'unchanged companion '+key+' retention allowance')
        require(after['heap_alloc']<=24*MIB,'unchanged companion heap hard budget')
        summary={'case':'separate-reasoning-storage-stress','shipping':shipping,'companion':result,'companion_before':before,'companion_after':after,'actual_records':len(files)-1,'actual_private_bytes':sum(sizes),'max_snapshot_bytes':max(sizes),'shipping_forced_gc':False,'companion_only_gc':True,'companion_measurement_coverage':companion.coverage}
        return summary
    finally:
        try:
            companion.close()
            if summary is not None:
                summary['companion_sampler_joined']=not companion.thread.is_alive()
        finally:
            for index,path in enumerate(companion.logs):
                shutil.copyfile(path,pathlib.Path('/evidence')/('companion-'+str(index)+'.log'))


def inner(args):
    require(os.environ.get('ATUIN_REASONING_NAMESPACE')=='1' and pathlib.Path('/work').is_dir(),'confined-only driver')
    os.umask(0o077)
    devices=[line.split(':')[0].strip() for line in pathlib.Path('/proc/net/dev').read_text().splitlines()[2:]]
    require(devices==['lo'] and len(pathlib.Path('/proc/net/route').read_text().splitlines())==1,'loopback-only namespace without remote routes')
    with socket.socket() as probe:
        probe.settimeout(.2);require(probe.connect_ex(('192.0.2.1',443))!=0,'no-egress negative witness')
    work=pathlib.Path('/work');tls=work/'tls';tls.mkdir(mode=0o700);certificates(tls)
    now=int(time.time());claims={'exp':now+7200,'https://api.openai.com/auth':{'chatgpt_account_id':'synthetic-reasoning-account'}}
    token='e30.'+base64.urlsafe_b64encode(compact(claims)).rstrip(b'=').decode()+'.c2ln';fake={'token':token,'account':'synthetic-reasoning-account'};(work/'fake.json').write_bytes(compact(fake));(work/'fake.json').chmod(0o600)
    env={'PATH':'/usr/bin:/bin','LANG':'C.UTF-8','HOME':'/work/home','XDG_CONFIG_HOME':'/work/config',
         'SSL_CERT_FILE':str(tls/'ca.pem'),'NO_PROXY':'127.0.0.1,localhost'}
    (work/'server.toml').write_text('port = 17391\ndefault_model = "gpt-6.1-sol"\n')
    subprocess.run(['/helper','setup',str(work/'fake.json')],env=env,check=True,capture_output=True,timeout=10)
    credentials=work/'config'/'atuin-ai'/'credentials'/'record.json'
    fixture=ReasoningFixture(tls,fake);env['HTTPS_PROXY']='http://127.0.0.1:'+str(fixture.port)
    server=None
    try:
        server=Server(env,work)
        server.start();summary=stress_proof(server,fixture,work) if args.stress else cli_proof(server,fixture,work,credentials)
        if not args.stress:
            server.stop();summary['packaged_init']=packaged_init_proof(work,env)
    finally:
        try:
            try:
                if server is not None:
                    server.close()
            finally:
                fixture.close()
        finally:
            for index,path in enumerate(server.logs if server is not None else []):
                shutil.copyfile(path,pathlib.Path('/evidence')/('server-'+str(index)+'.log'))
    summary.update({'max_shipping_rss_bytes':server.max_rss,'max_shipping_fds':server.max_fds,'measurement_coverage':server.coverage,'sampler_joined':not server.thread.is_alive(),'active_after_cleanup':fixture.active,'peak_native_active':fixture.peak_active,'read_only_inputs':list(INPUTS)})
    pathlib.Path('/evidence/result.json').write_bytes(json.dumps(summary,sort_keys=True,indent=2).encode()+b'\n')
    print('PASS separate reasoning storage/RSS/FD/companion stress' if args.stress else 'PASS separate actual pinned CLI reasoning/restart; 3 generations; 2 process restarts; zero catalogs')


def outer(args):
    repo=HERE.parent.parent
    originals=[pathlib.Path(p).absolute() for p in (args.server,args.helper,args.cli)]
    for path in originals:
        require(stat.S_ISREG(path.lstat().st_mode) and not path.is_symlink(),'regular non-symlink explicit proof binary')
    binaries=[path.resolve(strict=True) for path in originals]
    out=pathlib.Path(args.out).absolute();require(not out.exists(),'fresh reasoning evidence directory required');out.mkdir(mode=0o700,parents=True)
    bwrap=resolve()
    with tempfile.TemporaryDirectory(prefix='atuin-reasoning-owned-') as temporary:
        temp=pathlib.Path(temporary);source=temp/'source';source.mkdir(mode=0o700);work=temp/'work';work.mkdir(mode=0o700);client=temp/'client';client.mkdir(mode=0o700);inventory=[]
        for name in INPUTS:
            path=repo/name;info=path.lstat();require(stat.S_ISREG(info.st_mode) and info.st_nlink==1 and not path.is_symlink(),'closed public source input')
            for parent in path.parents:
                require(not parent.is_symlink(),'source ancestor symlink')
                if parent==repo:
                    break
            target=source/name;target.parent.mkdir(mode=0o700,parents=True,exist_ok=True);shutil.copyfile(path,target);target.chmod(0o444);inventory.append({'path':name,'sha256':digest(target),'bytes':info.st_size})
        descriptors=[]
        def pin(path):
            fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW|os.O_CLOEXEC);descriptors.append(fd);return '/proc/self/fd/'+str(fd)
        command=[str(bwrap),'--die-with-parent','--new-session','--unshare-user','--unshare-pid','--unshare-net','--tmpfs','/',
                 '--ro-bind','/usr','/usr','--symlink','usr/bin','/bin','--symlink','usr/lib','/lib','--symlink','usr/lib64','/lib64',
                 '--dir','/etc','--dir','/tmp','--ro-bind',pin(source),'/repo','--ro-bind',pin(binaries[0]),'/binary',
                 '--ro-bind',pin(binaries[1]),'/helper','--ro-bind',pin(binaries[2]),'/atuin','--bind',pin(work),'/work',
                 '--bind',pin(client),'/fixture','--bind',pin(out),'/evidence','--proc','/proc','--dev','/dev','--chdir','/repo',
                 '--clearenv','--setenv','PATH','/usr/bin:/bin','--setenv','PYTHONDONTWRITEBYTECODE','1','--setenv','ATUIN_REASONING_NAMESPACE','1',
                 '/usr/bin/python3','-B','-I','/repo/tests/reasoning/run.py','--inside']
        if args.stress:
            command.append('--stress')
        try:
            completed=subprocess.run(command,capture_output=True,timeout=300,pass_fds=tuple(descriptors))
        finally:
            for fd in descriptors:
                os.close(fd)
        (out/'namespace.log').write_bytes(completed.stdout+completed.stderr)
        (out/'inputs.json').write_bytes(json.dumps({'sources':inventory,'binaries':[{'path':str(p),'sha256':digest(p)} for p in binaries]},sort_keys=True,indent=2).encode()+b'\n')
        require(completed.returncode==0,'confined reasoning proof failed; see '+str(out/'namespace.log'))
        print(completed.stdout.decode().strip())


def main():
    parser=argparse.ArgumentParser();parser.add_argument('--inside',action='store_true');parser.add_argument('--stress',action='store_true');parser.add_argument('--server');parser.add_argument('--helper');parser.add_argument('--cli');parser.add_argument('--out');args=parser.parse_args()
    if args.inside:
        inner(args)
    else:
        require(all((args.server,args.helper,args.cli,args.out)),'explicit owned binaries/evidence required');outer(args)


if __name__=='__main__':
    main()
