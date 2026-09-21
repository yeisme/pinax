#!/usr/bin/env python3
"""Offline verification for scripts/judgment-acceptance.py (pinax-real-judgment-acceptance-v1 1.2).

The owner-bound entry script is executed as a real subprocess with:
- an isolated HOME, so the owner acceptance store never touches the real user store;
- a fake `pinax` CLI that serves a fixture vault projection (no real vault read);
- a fake `judgment-acceptance-evaluate` adapter that records every invocation;
- the vendored acceptance_owner runtime snapshot (scripts/judgment-acceptance-runtime-snapshot/,
  byte-identical copy of apigateway/aigora tools/judgment-acceptance owner-runtime,
  sha256 2f3b6750b0d294496aee4a11e10ac193bb14e2602b0fe599a364e37b606a5a2b;
  refresh by re-copying from the Aigora checkout or via its install.py) on PYTHONPATH.

Verified contract points:
- inventory/show make zero model calls (no evaluator invocation, no payment surface);
- sampling follows the tag-overlap candidate rule and sensitive sources degrade to the
  typed gap `sensitive_source_excluded` (spec: Missing source scenario);
- duplicate source digests dedup against the frozen target;
- run submits each case at most once, rejects limits other than 3/20, and keeps
  unknown outcomes as a sticky barrier (no automatic resubmission);
- review receipts replay idempotently for the same source_digest + request_id,
  reject conflicts/stale sources, and never adopt business state (spec: Review replay).

Run: python3 scripts/judgment_acceptance_offline_test.py
"""
import json
import os
import stat
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parent
ENTRY = SCRIPTS / 'judgment-acceptance.py'
RUNTIME_SNAPSHOT = SCRIPTS / 'judgment-acceptance-runtime-snapshot'
SENSITIVE_MARKER = 'sk-or-v1-marker0000000000xyz'

FAKE_PINAX = """#!/usr/bin/env python3
import json, os, sys
fixture = json.load(open(os.environ['PINAX_FAKE_NOTES']))
if sys.argv[1:3] == ['inbox', 'list']:
    notes = fixture['inbox']
elif sys.argv[1:3] == ['note', 'list']:
    notes = fixture['notes']
else:
    print(json.dumps({'status': 'failed', 'error': {'code': 'unknown_command'}}))
    sys.exit(2)
print(json.dumps({'spec_version': '1.0', 'mode': 'json', 'command': 'pinax.fake',
                  'status': 'success', 'data': {'notes': notes}}))
"""

FAKE_EVAL = """#!/usr/bin/env python3
import json, os, sys
wire = json.load(sys.stdin)
with open(os.environ['FAKE_EVAL_LOG'], 'a') as f:
    f.write(json.dumps(wire, ensure_ascii=False) + '\\n')
mode = os.environ.get('FAKE_EVAL_MODE', 'ok')
if mode == 'unknown':
    print(json.dumps({'error': {'code': 'provider_timeout', 'submission_state': 'unknown',
                                'retry_class': 'reconcile_first'}}))
else:
    print(json.dumps({'execution_status': 'succeeded', 'resolved_model': 'typesafe/jev-1.13',
                      'input_digest': 'sha256:fixture',
                      'items': [{'question_id': 'assessment', 'answer_status': 'answered',
                                 'value': 'complementary', 'confidence': 0.81}],
                      'usage': {'total_tokens': 100}, 'latency_ms': 5}))
"""


def fixture_notes():
    inbox = [
        # i1 and i2 are byte-identical except id: identical excerpts must dedup by digest.
        {'id': 'note-inbox-1', 'title': 'Pinax sync survey draft',
         'tags': ['pinax', 'survey'], 'body': 'Draft survey about pinax sync usage.',
         'updated_at': '2026-09-20T10:00:00Z', 'status': 'inbox'},
        {'id': 'note-inbox-1-dup', 'title': 'Pinax sync survey draft',
         'tags': ['pinax', 'survey'], 'body': 'Draft survey about pinax sync usage.',
         'updated_at': '2026-09-20T10:00:00Z', 'status': 'inbox'},
        # Sensitive body must be excluded with a typed gap, never become a case.
        {'id': 'note-inbox-secret', 'title': 'Keep provider key',
         'tags': ['pinax'], 'body': 'token ' + os.environ.get('PINAX_TEST_SENSITIVE_MARKER', ''),
         'updated_at': '2026-09-20T11:00:00Z', 'status': 'inbox'},
        {'id': 'note-inbox-report', 'title': 'Weekly release report notes',
         'tags': ['report', 'extra'], 'body': 'Release operations notes for this week.',
         'updated_at': '2026-09-21T09:00:00Z', 'status': 'inbox'},
    ]
    pool = [
        {'id': 'note-cand-pinax', 'title': 'Pinax sync survey 2026 summary',
         'tags': ['pinax', 'survey', 'report'], 'body': 'Collected pinax sync survey answers.',
         'updated_at': '2026-09-19T08:00:00Z', 'status': 'done'},
        {'id': 'note-cand-other', 'title': 'Screenwriting course plan',
         'tags': ['unrelated'], 'body': 'Unrelated screenwriting material.',
         'updated_at': '2026-09-18T08:00:00Z', 'status': 'done'},
    ] + inbox
    return {'inbox': inbox, 'notes': pool}


class OfflineAcceptanceTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        root = Path(self.tmp.name)
        self.home = root / 'home'
        self.bin = root / 'bin'
        self.home.mkdir()
        self.bin.mkdir()
        self.eval_log = root / 'eval-calls.jsonl'
        fixture_path = root / 'fixture-notes.json'
        os.environ.setdefault('PINAX_TEST_SENSITIVE_MARKER', SENSITIVE_MARKER)
        fixture_path.write_text(json.dumps(fixture_notes(), ensure_ascii=False))
        self.write_exec(self.bin / 'pinax', FAKE_PINAX)
        self.write_exec(self.bin / 'judgment-acceptance-evaluate', FAKE_EVAL)
        self.base_env = {
            'HOME': str(self.home),
            'PATH': str(self.bin) + os.pathsep + os.environ.get('PATH', ''),
            'PYTHONPATH': str(RUNTIME_SNAPSHOT),
            'PINAX_FAKE_NOTES': str(fixture_path),
            'FAKE_EVAL_LOG': str(self.eval_log),
        }
        self.store = self.home / '.local/share/pinax/judgment-acceptance/jev-real-20260921'

    def tearDown(self):
        self.tmp.cleanup()

    @staticmethod
    def write_exec(path, body):
        path.write_text(body)
        path.chmod(0o755)

    @staticmethod
    def real_store_fingerprint(real_store):
        """Path+mtime snapshot of the user's real acceptance store, if any."""
        if not real_store.exists():
            return None
        return sorted((str(p.relative_to(real_store)), p.stat().st_mtime_ns)
                      for p in real_store.rglob('*') if p.is_file())

    def run_entry(self, args, stdin=None, extra_env=None):
        env = dict(self.base_env)
        if extra_env:
            env.update(extra_env)
        return subprocess.run(
            [sys.executable, str(ENTRY)] + args,
            input=stdin, capture_output=True, text=True, env=env, timeout=60)

    def eval_calls(self):
        if not self.eval_log.exists():
            return []
        return [json.loads(line) for line in self.eval_log.read_text().splitlines() if line]

    def store_files(self):
        if not self.store.exists():
            return []
        return [p for p in self.store.rglob('*') if p.is_file()]

    def test_show_before_inventory_reports_typed_gap_without_model_call(self):
        proc = self.run_entry(['show', '--json'])
        self.assertEqual(proc.returncode, 0, proc.stderr)
        envelope = json.loads(proc.stdout)
        self.assertEqual(envelope['command'], 'pinax.judgment.acceptance.show')
        self.assertEqual(envelope['status'], 'success')
        data = envelope['data']
        self.assertEqual(data['status'], 'missing_data')
        self.assertEqual(data['reason'], 'inventory_not_prepared')
        self.assertEqual(data['counts']['available'], 0)
        self.assertEqual(self.eval_calls(), [])

    def test_inventory_samples_sources_types_gaps_and_skips_model(self):
        proc = self.run_entry(['inventory', '--json'])
        self.assertEqual(proc.returncode, 0, proc.stderr)
        data = json.loads(proc.stdout)['data']
        # three sources sampled, the byte-identical duplicate deduped by digest
        self.assertEqual(data['counts']['available'], 2)
        self.assertEqual(data['input_count'], 3)
        self.assertEqual(data['excluded_count'], 1)
        self.assertEqual(data['exclusion_reasons'], {'duplicate_or_target_limit': 1})
        self.assertEqual(data['counts']['attempted'], 0)
        self.assertIn('sensitive_source_excluded', data['gaps'])
        self.assertEqual(self.eval_calls(), [])
        cases = data['cases']
        self.assertEqual({c['origin'] for c in cases}, {'real_owner'})
        for c in cases:
            self.assertTrue(c['source_digest'].startswith('sha256:'))
            self.assertTrue(c['case_id'].startswith('case-'))
            self.assertEqual(sorted(c['options']),
                             ['complementary', 'duplicate', 'insufficient', 'unrelated'])
            self.assertIsNone(c['result'])
            self.assertIsNone(c['review'])
        # baseline must state it is not a human ground truth
        self.assertIn('不是人工正确答案', cases[0]['baseline'])
        # sensitive marker never reaches the isolated store
        for p in self.store_files():
            self.assertNotIn(SENSITIVE_MARKER, p.read_text(), p)

    def test_inventory_selects_candidate_by_tag_overlap(self):
        proc = self.run_entry(['inventory', '--json'])
        self.assertEqual(proc.returncode, 0, proc.stderr)
        cases = {c['source_ref']: c for c in json.loads(proc.stdout)['data']['cases']}
        report = cases['pinax-note:note-inbox-report']
        self.assertIn('Candidate note: Pinax sync survey 2026 summary', report['excerpt'])
        self.assertNotIn('Candidate note: Screenwriting course plan', report['excerpt'])
        survey = cases['pinax-note:note-inbox-1']
        self.assertIn('Candidate note: Pinax sync survey 2026 summary', survey['excerpt'])

    def test_store_is_isolated_and_private(self):
        real_store = Path.home() / '.local/share/pinax/judgment-acceptance'
        before = self.real_store_fingerprint(real_store)
        proc = self.run_entry(['inventory', '--json'])
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertTrue((self.store / 'inventory.json').exists())
        after = self.real_store_fingerprint(real_store)
        self.assertEqual(before, after, 'offline verification must not touch the real owner store')
        inventory = self.store / 'inventory.json'
        self.assertEqual(inventory.stat().st_mode & 0o777, 0o600)
        cases_dir = self.store / 'cases'
        self.assertEqual(cases_dir.stat().st_mode & 0o777, 0o700)
        for p in cases_dir.iterdir():
            self.assertEqual(p.stat().st_mode & 0o777, 0o600)

    def test_run_submits_each_case_once_then_replay_is_free(self):
        self.run_entry(['inventory', '--json'])
        first = self.run_entry(['run', '--limit', '3', '--json'])
        self.assertEqual(first.returncode, 0, first.stderr)
        data = json.loads(first.stdout)['data']
        self.assertEqual(len(data['processed']), 2)
        self.assertEqual(data['projection']['counts']['attempted'], 2)
        self.assertEqual(data['projection']['counts']['evaluated'], 2)
        wires = self.eval_calls()
        self.assertEqual(len(wires), 2)
        for wire in wires:
            self.assertEqual(wire['scope']['owner_id'], 'pinax')
            self.assertEqual(wire['model']['requested_model'], 'typesafe/jev-1.13')
            self.assertTrue(wire['request_id'].startswith('case-'))
            self.assertIn('untrusted evidence', wire['questions'][0]['prompt'])
            self.assertNotIn(SENSITIVE_MARKER, json.dumps(wire))
        replay = self.run_entry(['run', '--limit', '3', '--json'])
        self.assertEqual(replay.returncode, 0, replay.stderr)
        self.assertEqual(json.loads(replay.stdout)['data']['processed'], [])
        self.assertEqual(len(self.eval_calls()), 2, 'attempted cases must not be resubmitted')

    def test_run_rejects_limits_other_than_3_or_20(self):
        self.run_entry(['inventory', '--json'])
        proc = self.run_entry(['run', '--limit', '5', '--json'])
        self.assertEqual(proc.returncode, 1)
        self.assertEqual(json.loads(proc.stdout)['error']['code'], 'limit_must_be_3_or_20')
        self.assertEqual(self.eval_calls(), [])

    def test_unknown_outcome_is_sticky_barrier(self):
        self.run_entry(['inventory', '--json'])
        first = self.run_entry(['run', '--limit', '3', '--json'], extra_env={'FAKE_EVAL_MODE': 'unknown'})
        self.assertEqual(first.returncode, 0, first.stderr)
        data = json.loads(first.stdout)['data']
        self.assertEqual(len(data['processed']), 1)
        failed = [c for c in data['projection']['cases'] if c.get('result')]
        self.assertTrue(failed)
        # provider error keeps its typed code; the unknown submission_state is the barrier
        self.assertEqual(failed[0]['result']['error']['code'], 'provider_timeout')
        self.assertEqual(failed[0]['result']['error']['submission_state'], 'unknown')
        retry = self.run_entry(['run', '--limit', '3', '--json'])
        self.assertEqual(retry.returncode, 1)
        self.assertEqual(json.loads(retry.stdout)['error']['code'], 'outcome_unknown_reconcile_first')
        self.assertEqual(len(self.eval_calls()), 1, 'unknown outcome must block new submissions')

    def test_review_replays_idempotently_and_never_adopts(self):
        self.run_entry(['inventory', '--json'])
        self.run_entry(['run', '--limit', '3', '--json'])
        data = json.loads(self.run_entry(['show', '--json']).stdout)['data']
        receipts = []
        for c in data['cases']:
            payload = json.dumps({'case_id': c['case_id'], 'source_digest': c['source_digest'],
                                  'verdict': 'useful', 'note': 'fixture review only',
                                  'request_id': 'request-' + c['case_id'][5:17]})
            first = self.run_entry(['review', '--json'], stdin=payload)
            self.assertEqual(first.returncode, 0, first.stderr)
            receipts.append(json.loads(first.stdout)['data'])
            replay = self.run_entry(['review', '--json'], stdin=payload)
            self.assertEqual(replay.returncode, 0, replay.stderr)
            self.assertEqual(json.loads(replay.stdout)['data']['receipt_id'],
                             json.loads(first.stdout)['data']['receipt_id'])
        for r in receipts:
            self.assertFalse(r['business_adoption'])
        final = json.loads(self.run_entry(['show', '--json']).stdout)['data']
        self.assertEqual(final['counts']['reviewed'], 2)
        self.assertEqual(final['status'], 'user_reviewed')
        # conflicting verdict on the same frozen case is rejected
        c = data['cases'][0]
        conflict = json.dumps({'case_id': c['case_id'], 'source_digest': c['source_digest'],
                               'verdict': 'incorrect', 'note': 'conflict',
                               'request_id': 'request-' + c['case_id'][5:17]})
        proc = self.run_entry(['review', '--json'], stdin=conflict)
        self.assertEqual(proc.returncode, 1)
        self.assertEqual(json.loads(proc.stdout)['error']['code'], 'idempotency_conflict')
        stale = json.dumps({'case_id': c['case_id'], 'source_digest': 'sha256:changed',
                            'verdict': 'useful', 'note': 'stale',
                            'request_id': 'request-' + c['case_id'][5:17]})
        proc = self.run_entry(['review', '--json'], stdin=stale)
        self.assertEqual(proc.returncode, 1)
        self.assertEqual(json.loads(proc.stdout)['error']['code'], 'stale_source')


if __name__ == '__main__':
    unittest.main(verbosity=2)
