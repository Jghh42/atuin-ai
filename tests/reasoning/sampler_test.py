#!/usr/bin/env python3
"""Fast synthetic coordinator tests; no process/native/network/store operations."""
import importlib.util
import pathlib
import sys
import tempfile
import threading
import types
import unittest
from unittest import mock

HERE=pathlib.Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('reasoning_runner',HERE/'run.py')
runner=importlib.util.module_from_spec(spec);spec.loader.exec_module(runner)
class OwnedFake:
    pid=99999991
    stdin=None
    stdout=None
    def send_signal(self,_):pass
    def wait(self,timeout):return 0
    def kill(self):pass
    def poll(self):return None
class Sampler(unittest.TestCase):
    def server(self):
        server=runner.Server({},pathlib.Path('/not-opened-owned-fixture'));self.addCleanup(lambda: (server.stop_sample.set(),server.thread.join(1)))
        return server
    def fault(self,error):
        entered=threading.Event()
        def read(_):entered.set();raise error
        with mock.patch.object(pathlib.Path,'read_text',read):
            server=self.server();server.process=OwnedFake();self.assertTrue(entered.wait(1))
            with self.assertRaises(AssertionError):server.close()
    def test_permission_error_propagates(self):self.fault(PermissionError('synthetic'))
    def test_read_failure_propagates(self):self.fault(OSError('synthetic'))
    def test_early_exit_propagates(self):self.fault(FileNotFoundError('synthetic'))
    def test_zero_samples_cannot_pass(self):
        server=self.server();server.stop_sample.set();server.thread.join(1);server.process=OwnedFake()
        with self.assertRaises(AssertionError):server.close()
    def test_join_must_finish(self):
        server=self.server();server.stop_sample.set();server.thread.join(1)
        server.thread=types.SimpleNamespace(join=lambda _:None,is_alive=lambda:True)
        with self.assertRaises(AssertionError):server.close()
    def test_nonzero_without_baseline_cannot_pass(self):
        server=self.server();server.stop_sample.set();server.thread.join(1);server.process=OwnedFake()
        with mock.patch.object(server,'read_measurement',return_value=(2048,3)):
            with server.sample_lock:server.observe_locked('periodic')
            with self.assertRaises(AssertionError):server.close()
    def test_final_sample_failure_cannot_pass(self):
        server=self.server();server.stop_sample.set();server.thread.join(1);server.process=OwnedFake()
        with mock.patch.object(server,'read_measurement',return_value=(2048,3)):server.mark_baseline()
        with mock.patch.object(server,'read_measurement',side_effect=PermissionError('synthetic')):
            with self.assertRaises(AssertionError):server.close()
    def test_missing_vm_hwm_cannot_pass(self):
        server=self.server();server.stop_sample.set();server.thread.join(1);server.process=OwnedFake()
        with mock.patch.object(pathlib.Path,'read_text',return_value='VmRSS: 1024 kB\n'):
            with self.assertRaises(AssertionError):server.close()
    def test_valid_read_and_cleanup_control(self):
        entered=threading.Event()
        def read(_):entered.set();return 'VmRSS:\t1024 kB\nVmHWM:\t2048 kB\n'
        with mock.patch.object(pathlib.Path,'read_text',read),mock.patch.object(pathlib.Path,'iterdir',return_value=iter([])):
            server=self.server();server.process=OwnedFake();self.assertTrue(entered.wait(1));server.mark_baseline();server.close();self.assertGreater(server.max_rss,0);self.assertFalse(server.thread.is_alive())
if __name__=='__main__':unittest.main()
