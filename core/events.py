"""Simple pub/sub event bus for decoupling core and GUI."""
from collections import defaultdict
from typing import Callable, Any


class EventBus:
    def __init__(self):
        self._subscribers: dict[str, list[Callable]] = defaultdict(list)

    def subscribe(self, event: str, callback: Callable[..., Any]) -> None:
        self._subscribers[event].append(callback)

    def emit(self, event: str, *args, **kwargs) -> None:
        for callback in self._subscribers.get(event, []):
            callback(*args, **kwargs)


bus = EventBus()
