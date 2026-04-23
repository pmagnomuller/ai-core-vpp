from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class TelemetryPoint(_message.Message):
    __slots__ = ("ts", "kw_usage", "battery_soc_pct", "heatpump_status", "grid_price_eur_kwh")
    TS_FIELD_NUMBER: _ClassVar[int]
    KW_USAGE_FIELD_NUMBER: _ClassVar[int]
    BATTERY_SOC_PCT_FIELD_NUMBER: _ClassVar[int]
    HEATPUMP_STATUS_FIELD_NUMBER: _ClassVar[int]
    GRID_PRICE_EUR_KWH_FIELD_NUMBER: _ClassVar[int]
    ts: str
    kw_usage: float
    battery_soc_pct: float
    heatpump_status: str
    grid_price_eur_kwh: float
    def __init__(self, ts: _Optional[str] = ..., kw_usage: _Optional[float] = ..., battery_soc_pct: _Optional[float] = ..., heatpump_status: _Optional[str] = ..., grid_price_eur_kwh: _Optional[float] = ...) -> None: ...

class OptimizeRequest(_message.Message):
    __slots__ = ("device_id", "history")
    DEVICE_ID_FIELD_NUMBER: _ClassVar[int]
    HISTORY_FIELD_NUMBER: _ClassVar[int]
    device_id: str
    history: _containers.RepeatedCompositeFieldContainer[TelemetryPoint]
    def __init__(self, device_id: _Optional[str] = ..., history: _Optional[_Iterable[_Union[TelemetryPoint, _Mapping]]] = ...) -> None: ...

class Action(_message.Message):
    __slots__ = ("when", "what", "why")
    WHEN_FIELD_NUMBER: _ClassVar[int]
    WHAT_FIELD_NUMBER: _ClassVar[int]
    WHY_FIELD_NUMBER: _ClassVar[int]
    when: str
    what: str
    why: str
    def __init__(self, when: _Optional[str] = ..., what: _Optional[str] = ..., why: _Optional[str] = ...) -> None: ...

class OptimizeResponse(_message.Message):
    __slots__ = ("strategy", "actions")
    STRATEGY_FIELD_NUMBER: _ClassVar[int]
    ACTIONS_FIELD_NUMBER: _ClassVar[int]
    strategy: str
    actions: _containers.RepeatedCompositeFieldContainer[Action]
    def __init__(self, strategy: _Optional[str] = ..., actions: _Optional[_Iterable[_Union[Action, _Mapping]]] = ...) -> None: ...
