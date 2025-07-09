from copy import deepcopy
from aiohttp import ClientSession
from typing import Tuple, Union, Any

from src.enums import RequestDatabaseType
from .exceptions import ClientNotInitialized, InvalidDatabaseType


class HarukiDBClient(object):
    def __init__(
        self,
        db_api: str,
        suite_api: str,
        db_api_authorization_token: str,
        suite_api_authorization_token: str,
        user_agent: str,
        haruki_cloud_version: str,
    ) -> None:
        self._db_api = db_api
        self._suite_api = suite_api
        self._headers = {
            "User-Agent": f"{user_agent}/v{haruki_cloud_version}",
            "Content-Type": "application/json",
            "accept": "application/json",
        }
        self._db_api_authorization_token = db_api_authorization_token
        self._suite_api_authorization_token = suite_api_authorization_token
        self._session: ClientSession | None = None

    async def init(self) -> None:
        self._session = ClientSession()

    async def close(self) -> None:
        if self._session:
            await self._session.close()

    async def call_api(
        self,
        path: str,
        method: str = "GET",
        data: Union[dict, list] = None,
        params: dict = None,
        db_type: RequestDatabaseType = RequestDatabaseType.main,
    ) -> Tuple[Any, int]:
        if self._session:
            if db_type == RequestDatabaseType.main:
                token = self._db_api_authorization_token
                api = self._db_api
            elif db_type == RequestDatabaseType.suite:
                token = self._suite_api_authorization_token
                api = self._suite_api
            else:
                raise InvalidDatabaseType
            headers = deepcopy(self._headers)
            headers["Authorization"] = f"Bearer {token}"
            async with self._session.request(
                method=method, url=api + path, params=params, json=data, headers=headers
            ) as response:
                return await response.json(), response.status
        else:
            raise ClientNotInitialized
