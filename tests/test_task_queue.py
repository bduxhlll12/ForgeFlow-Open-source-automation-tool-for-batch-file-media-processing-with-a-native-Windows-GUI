"""Unit tests for core.task_queue."""
import unittest
from core.task_queue import TaskQueue, Task


class TestTaskQueue(unittest.TestCase):
    def test_submit_runs_function(self):
        queue = TaskQueue(max_workers=1)
        result = queue.submit(Task(task_id="t1", func=lambda: 2 + 2)).result()
        self.assertEqual(result, 4)
        queue.shutdown()


if __name__ == "__main__":
    unittest.main()
