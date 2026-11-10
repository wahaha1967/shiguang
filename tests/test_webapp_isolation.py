"""Regression tests for ownership and transaction boundaries."""

import sys
import unittest
from pathlib import Path

from sqlalchemy import create_engine
from sqlalchemy.orm import Session

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'src'))

from shiguang_study.webapp import db, ops  # noqa: E402


class OwnershipTests(unittest.TestCase):
    def setUp(self) -> None:
        self.engine = create_engine('sqlite:///:memory:')
        db.Base.metadata.create_all(self.engine)
        self.session = Session(self.engine)

    def tearDown(self) -> None:
        self.session.close()
        self.engine.dispose()

    def test_upsert_cannot_take_another_owners_record(self) -> None:
        cases = [
            ('questions', {'id': 'same', 'title': '原题', 'answer': '答案',
                           'due': '2026-11-10', 'created': '2026-11-10'}),
            ('logs', {'id': 'same', 'date': '2026-11-10', 'type': 'add'}),
            ('rewards', {'id': 'same', 'name': '奖励', 'cost': 1}),
            ('redemptions', {'id': 'same', 'name': '奖励', 'cost': 1,
                             'date': '2026-11-10'}),
            ('aiSets', {'id': 'same', 'created': '2026-11-10',
                        'questions': [{'id': f'item-{index}'} for index in range(3)]}),
        ]
        for kind, row in cases:
            with self.subTest(kind=kind):
                operation = {'t': kind, 'op': 'upsert', 'row': row}
                ops.apply_ops(self.session, 'owner-a', [operation])
                self.session.commit()
                with self.assertRaises(ops.InvalidOperation):
                    ops.apply_ops(self.session, 'owner-b', [operation])
                self.session.rollback()
                self.assertEqual(len(ops.read_state(self.session, 'owner-a')[kind]), 1)
                self.assertEqual(len(ops.read_state(self.session, 'owner-b')[kind]), 0)

    def test_owner_can_update_existing_question(self) -> None:
        row = {'id': 'question-1', 'title': '原题', 'answer': '答案',
               'due': '2026-11-10', 'created': '2026-11-10'}
        operation = {'t': 'questions', 'op': 'upsert', 'row': row}
        ops.apply_ops(self.session, 'owner-a', [operation])
        self.session.commit()
        row['title'] = '修改后的题目'
        ops.apply_ops(self.session, 'owner-a', [operation])
        self.session.commit()
        self.assertEqual(ops.read_state(self.session, 'owner-a')['questions'][0]['title'],
                         '修改后的题目')

    def test_import_with_existing_ai_set_replaces_successfully(self) -> None:
        ai_set = {'id': 'set-1', 'created': '2026-11-10',
                  'questions': [{'id': f'item-{index}'} for index in range(3)]}
        ops.apply_ops(self.session, 'owner-a', [{'t': 'aiSets', 'op': 'upsert', 'row': ai_set}])
        self.session.commit()
        ops.replace_state(self.session, 'owner-a', {'schema': 1, 'questions': [], 'logs': [],
                                                   'prefs': {'name': '同学'}})
        self.session.commit()
        self.assertEqual(ops.read_state(self.session, 'owner-a')['aiSets'], [])

    def test_invalid_import_does_not_delete_existing_data(self) -> None:
        row = {'id': 'original', 'title': '原题', 'answer': '答案',
               'due': '2026-11-10', 'created': '2026-11-10'}
        ops.apply_ops(self.session, 'owner-a', [{'t': 'questions', 'op': 'upsert', 'row': row}])
        self.session.commit()
        with self.assertRaises(ops.InvalidOperation):
            ops.replace_state(self.session, 'owner-a', {'schema': 1, 'questions': [None],
                                                       'logs': [], 'prefs': {'name': '同学'}})
        self.session.rollback()
        self.assertEqual(ops.read_state(self.session, 'owner-a')['questions'][0]['title'], '原题')

    def test_incomplete_import_is_rejected(self) -> None:
        with self.assertRaises(ops.InvalidOperation):
            ops.replace_state(self.session, 'owner-a', {'questions': []})

    def test_failed_replacement_rolls_back_deletions(self) -> None:
        row = {'id': 'original', 'title': '原题', 'answer': '答案',
               'due': '2026-11-10', 'created': '2026-11-10'}
        ops.apply_ops(self.session, 'owner-a', [{'t': 'questions', 'op': 'upsert', 'row': row}])
        self.session.commit()
        invalid = {**row, 'stage': 10}
        with self.assertRaises(ops.InvalidOperation):
            ops.replace_state(self.session, 'owner-a', {'schema': 1, 'questions': [invalid],
                                                       'logs': [], 'prefs': {'name': '同学'}})
        self.session.rollback()
        self.assertEqual(ops.read_state(self.session, 'owner-a')['questions'][0]['title'], '原题')

    def test_failed_batch_rolls_back_previous_operations(self) -> None:
        row = {'id': 'new', 'title': '新题', 'answer': '答案',
               'due': '2026-11-10', 'created': '2026-11-10'}
        with self.assertRaises(ops.InvalidOperation):
            ops.apply_ops(self.session, 'owner-a', [
                {'t': 'questions', 'op': 'upsert', 'row': row},
                {'t': 'unknown', 'op': 'upsert'},
            ])
        self.session.rollback()
        self.assertEqual(ops.read_state(self.session, 'owner-a')['questions'], [])


if __name__ == '__main__':
    unittest.main()
