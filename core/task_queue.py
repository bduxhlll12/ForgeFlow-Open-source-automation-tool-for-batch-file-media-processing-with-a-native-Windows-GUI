"""Parallel task queue for processing jobs."""
from concurrent.futures import ThreadPoolExecutor, Future
from dataclasses import dataclass
from typing import Callable, Any


@dataclass
class Task:
    task_id: str
    func: Callable
    args: tuple = ()
    kwargs: dict = None


class TaskQueue:
    def __init__(self, max_workers: int = 4):
        self._executor = ThreadPoolExecutor(max_workers=max_workers)
        self._futures: dict[str, Future] = {}

    def submit(self, task: Task) -> Future:
        kwargs = task.kwargs or {}
        future = self._executor.submit(task.func, *task.args, **kwargs)
        self._futures[task.task_id] = future
        return future

    def shutdown(self) -> None:
        self._executor.shutdown(wait=True)
