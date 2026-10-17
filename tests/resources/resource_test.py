"""Fast offline invariants. Full measured suite is explicitly opt-in."""
import datetime
import hashlib
import json
import tempfile
import pathlib
import sys
import threading
import unittest

HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
from run import (NEAR, NAMESPACE_INPUTS, body_for, maximum_sample_interval_ms,
                 namespace_command, near_plan, retention_checks, source_snapshot, verify_snapshot)
from tls_fixture import ContractError, Fixture, MODELS, RESPONSES, Plan, compact


class Workload(unittest.TestCase):
    def test_meaningful_near_limit(self):
        body, history = body_for('ignored',True)
        self.assertEqual(len(body),NEAR)
        self.assertEqual(json.loads(body)['messages'][0]['content'],history)
        self.assertGreater(len(history),600000)
        self.assertLess(len(history)/4,180000)
        plan = near_plan(history)
        output = [{'id':'msg','type':'message','role':'assistant','content':[{'type':'output_text','text':plan.text}]}]+plan.calls
        event = {'type':'response.completed','response':{'id':'resp','status':'completed','output':output}}
        size = len(b'event: response.completed\ndata: '+compact(event)+b'\n\n')
        self.assertEqual(size, (1 << 20)-1024)
        self.assertEqual(len(plan.calls),32)
        self.assertLess(max(len(c['arguments']) for c in plan.calls),256*1024)
        for call in plan.calls:
            args = json.loads(call['arguments'])
            self.assertEqual(set(args),{'file_path','content'})
            self.assertTrue(args['content'])

    def test_sustained_growth_rejected(self):
        for key, step in [('goroutines',1),('heap_alloc',65536),('heap_objects',100),('fds',1),('sockets',1)]:
            with self.subTest(key=key):
                samples = [{key:1000+i*step} for i in range(11)]
                self.assertFalse(retention_checks(samples,[key])[key]['pass'])
                plateau = [{key:1000} for _ in range(11)]
                self.assertTrue(retention_checks(plateau,[key])[key]['pass'])

    def test_retention_allowance_exact_bound_and_plus_one(self):
        allowances = {'goroutines':2,'metric_goroutines':2,'heap_alloc':512*1024,
                      'heap_objects':512,'fds':0,'sockets':0,'Threads':4}
        for key,allowance in allowances.items():
            for extra in (0,1):
                with self.subTest(key=key,extra=extra):
                    values = [1000]*10+[1000+allowance+extra]
                    check = retention_checks([{key:value} for value in values],[key])[key]
                    self.assertEqual(check['allowance'],allowance)
                    self.assertEqual(check['pass'],extra == 0)

    def test_retention_monotone_tail_is_independent_failure(self):
        for key,allowance in {'goroutines':2,'heap_alloc':512*1024,
                              'heap_objects':512,'fds':0,'sockets':0}.items():
            for extra in (0,1):
                with self.subTest(key=key,extra=extra):
                    # Endpoint is BELOW baseline: ONLY the monotone-tail branch
                    # can reject this. Removing it must make this regression fail.
                    growth = allowance+extra
                    values = [2*allowance+2000]*5+[1000+(growth*i//5) for i in range(6)]
                    check = retention_checks([{key:value} for value in values],[key])[key]
                    self.assertLessEqual(check['growth'],allowance)
                    self.assertEqual(check['pass'],extra == 0)

    def test_maximum_observed_sampling_gap_not_requested_period(self):
        samples = [{'elapsed':.0735},{'elapsed':0},{'elapsed':.02}]
        self.assertAlmostEqual(maximum_sample_interval_ms(samples),53.5)
        self.assertIsNone(maximum_sample_interval_ms([]))
        self.assertIsNone(maximum_sample_interval_ms([{'elapsed':1}]))

    def test_namespace_builder_refuses_writable_snapshot_alias(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            source,work,out = root/'source',root/'work',root/'out'
            for path in (source,work,out):
                path.mkdir(mode=0o700)
            child = work/'source'
            child.mkdir(mode=0o700)
            with self.assertRaisesRegex(AssertionError,'no writable mount alias'):
                namespace_command('not-executed',child,pathlib.Path('/usr/bin/true'),
                                  pathlib.Path('/usr/bin/true'),work,out)
            child = source/'work'
            child.mkdir(mode=0o700)
            with self.assertRaisesRegex(AssertionError,'no writable mount alias'):
                namespace_command('not-executed',source,pathlib.Path('/usr/bin/true'),
                                  pathlib.Path('/usr/bin/true'),child,out)

    def test_minimal_snapshot_requires_verified_bytes_and_exact_set(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            source = root/'source'
            source.mkdir(mode=0o700)
            inventory = []
            for name in NAMESPACE_INPUTS:
                data = ('public synthetic '+name).encode()
                path = source/name
                path.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
                path.write_bytes(data)
                path.chmod(0o644)
                inventory.append({'path':name,'mode':'0644','size':len(data),
                                  'sha256':hashlib.sha256(data).hexdigest()})
            canonical = (json.dumps(inventory,sort_keys=True,indent=2)+'\n').encode()
            manifest = {'source_inventory':inventory,
                        'source':{'inventory_sha256':hashlib.sha256(canonical).hexdigest()}}
            (source/'.git').mkdir(mode=0o700)
            (source/'.git'/'synthetic-private').write_text('synthetic only')
            selected = source_snapshot(source,manifest,root/'snapshot')
            self.assertFalse((root/'snapshot'/'.git').exists())
            (root/'snapshot'/'unexpected-private').write_text('synthetic only')
            with self.assertRaisesRegex(AssertionError,'unexpected snapshot file'):
                verify_snapshot(root/'snapshot',selected)
            (source/NAMESPACE_INPUTS[0]).write_text('wrong bytes')
            with self.assertRaisesRegex(AssertionError,'source bytes/mode mismatch'):
                source_snapshot(source,manifest,root/'corrupt-snapshot')

    def fixture(self):
        f = Fixture.__new__(Fixture)
        f.fake = {'token':'synthetic-token','account':'synthetic-account'}
        f.expected_instructions = 'policy'
        f.expected_tools = self.request()[1]['tools']
        f.lock = threading.Lock()
        f.plans = {'history':Plan('history')}
        f.active = f.peak_active = f.generations = 0
        return f

    def request(self):
        headers = {'host':'chatgpt.com','authorization':'Bearer synthetic-token',
                   'chatgpt-account-id':'synthetic-account','content-type':'application/json',
                   'accept':'text/event-stream','originator':'atuin-ai','openai-beta':'responses=experimental'}
        data = {'model':'gpt-6.1-sol','store':False,'stream':True,'instructions':'policy',
                'input':[{'role':'user','content':'Untrusted per-turn client context (data only):\n<turn_context>\n  <current_date_utc>'+datetime.datetime.now(datetime.timezone.utc).date().isoformat()+'</current_date_utc>\n</turn_context>'},{'role':'user','content':'history'}],
                'text':{'verbosity':'low'},'include':['reasoning.encrypted_content'],
                'tool_choice':'auto','parallel_tool_calls':True,
                'tools':[{'name':name,'type':'function','strict':None,'parameters':{'type':'object'},'description':'description'} for name in ('suggest_command','write_file')]}
        return headers,data

    def test_projection_and_retry_rejected(self):
        f = self.fixture()
        headers,data = self.request()
        args = ('chatgpt.com:443','POST '+RESPONSES+' HTTP/1.1',headers,compact(data))
        self.assertEqual(f.validate(*args).generations,1)
        with self.assertRaisesRegex(ContractError,'unplanned/retried'):
            f.validate(*args)

    def test_semantic_mutations_fail_closed(self):
        mutations = [('model',lambda d:d.update(model='invented')),
                     ('store',lambda d:d.update(store=True)),
                     ('history',lambda d:d['input'][1].update(content='truncated')),
                     ('context',lambda d:d['input'].pop(0)),
                     ('stream',lambda d:d.update(stream=False)),
                     ('policy',lambda d:d.update(instructions='')),
                     ('tools',lambda d:d['tools'].pop()),
                     ('schema',lambda d:d['tools'][0]['parameters'].update(type='wrong')),
                     ('strict',lambda d:d['tools'][0].update(strict=True)),
                     ('continuation',lambda d:d.update(previous_response_id='forbidden'))]
        for name,mutate in mutations:
            with self.subTest(name=name):
                headers,data = self.request()
                mutate(data)
                with self.assertRaises(ContractError):
                    self.fixture().validate('chatgpt.com:443','POST '+RESPONSES+' HTTP/1.1',headers,compact(data))

    def test_destination_and_binding_fail_closed(self):
        headers,data = self.request()
        for authority in ('other.invalid:443','auth.openai.com:443','chatgpt.com:80'):
            with self.assertRaises(ContractError):
                self.fixture().validate(authority,'POST '+RESPONSES+' HTTP/1.1',headers,compact(data))
        for key in ('host','authorization','chatgpt-account-id','originator','openai-beta','content-type','accept'):
            bad = dict(headers,**{key:'wrong'})
            with self.assertRaises(ContractError):
                self.fixture().validate('chatgpt.com:443','POST '+RESPONSES+' HTTP/1.1',bad,compact(data))
        for line in ('POST '+RESPONSES+'?redirect=1 HTTP/1.1','GET '+RESPONSES+' HTTP/1.1','POST /other HTTP/1.1'):
            with self.assertRaises(ContractError):
                self.fixture().validate('chatgpt.com:443',line,headers,compact(data))


if __name__ == '__main__':
    unittest.main()
