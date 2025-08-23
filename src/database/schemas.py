from __future__ import annotations

from pydantic import BaseModel

from src.enums import SekaiBindingServerRegion


class PjskBindingRequest(BaseModel):
    server: SekaiBindingServerRegion | None = None
