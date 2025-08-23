from typing import Any, List

from src.enums import SekaiBindingServerRegion, InstantMessengerPlatform, PjskAliasType, SekaiSuiteDataType

from .client import HarukiDBClient
from .schemas import PjskBindingRequest


class HarukiDBOperator(HarukiDBClient):
    # ---------pjsk binding apis---------
    async def get_pjsk_binding(
        self, platform: InstantMessengerPlatform, im_id: str, server: SekaiBindingServerRegion = None
    ) -> tuple[Any, int]:
        return await self.call_api(
            path=f"/pjsk/{str(platform)}/user/{im_id}/binding",
            params=PjskBindingRequest(server=server).model_dump(exclude_none=True) if server else None,
        )

    async def add_pjsk_binding(
        self, platform: InstantMessengerPlatform, im_id: str, server: SekaiBindingServerRegion = None
    ) -> tuple[Any, int]: ...

    async def get_pjsk_default_binding(
        self,
        platform: InstantMessengerPlatform,
        im_id: str,
        server: SekaiBindingServerRegion = SekaiBindingServerRegion.default,
    ) -> tuple[Any, int]: ...

    async def set_pjsk_default_binding(
        self,
        platform: InstantMessengerPlatform,
        im_id: str,
        server: SekaiBindingServerRegion = SekaiBindingServerRegion.default,
    ) -> tuple[Any, int]: ...

    async def delete_pjsk_default_binding(
        self,
        platform: InstantMessengerPlatform,
        im_id: str,
        server: SekaiBindingServerRegion = SekaiBindingServerRegion.default,
    ) -> tuple[Any, int]: ...

    async def delete_pjsk_binding(
        self, platform: InstantMessengerPlatform, im_id: str, server: SekaiBindingServerRegion = None
    ) -> tuple[Any, int]: ...

    async def update_pjsk_binding(
        self, platform: InstantMessengerPlatform, im_id: str, server: SekaiBindingServerRegion = None
    ) -> tuple[Any, int]: ...

    # ------------------------------------
    # ---------pjsk alias apis------------
    async def query_object_id_by_alias(
        self, alias: str, alias_type: PjskAliasType = PjskAliasType.music, group_id: int = None
    ) -> tuple[Any, int]: ...

    async def get_all_aliases(
        self, alias_id: int, alias_type: PjskAliasType = PjskAliasType.music, group_id: int = None
    ) -> tuple[Any, int]: ...

    async def add_alias(
        self, alias_id: int, alias_type: PjskAliasType = PjskAliasType.music, group_id: int = None
    ) -> tuple[Any, int]: ...

    async def delete_alias(
        self, alias_id: int, alias_type: PjskAliasType = PjskAliasType.music, group_id: int = None
    ) -> tuple[Any, int]: ...

    async def get_pending_review_aliases(self) -> tuple[Any, int]: ...

    async def approve_alias(self, pending_review_id: int) -> tuple[Any, int]: ...

    async def reject_alias(self, pending_review_id: int, reason: str = None) -> tuple[Any, int]: ...

    async def get_pending_review_alias_status(self, pending_review_id: int) -> tuple[Any, int]: ...

    # -------------------------------------
    # ------pjsk user preferences apis-----

    async def get_all_preferences(self, platform: InstantMessengerPlatform, im_id: str) -> tuple[Any, int]: ...

    async def get_specific_preference(
        self, platform: InstantMessengerPlatform, im_id: str, option: str
    ) -> tuple[Any, int]: ...

    async def update_specific_preference(
        self, platform: InstantMessengerPlatform, im_id: str, option: str, value: str
    ) -> tuple[Any, int]: ...

    async def delete_specific_preference(
        self, platform: InstantMessengerPlatform, im_id: str, option: str
    ) -> tuple[Any, int]: ...

    # -------------------------------------
    # --------chunithm binding apis--------
    async def get_chunithm_default_server(self, platform: InstantMessengerPlatform, im_id: str) -> tuple[Any, int]: ...

    async def set_chunithm_default_server(
        self, platform: InstantMessengerPlatform, im_id: str, server: str
    ) -> tuple[Any, int]: ...

    async def delete_chunithm_default_server(
        self, platform: InstantMessengerPlatform, im_id: str, server: str
    ) -> tuple[Any, int]: ...

    async def get_chunithm_binding(
        self, platform: InstantMessengerPlatform, im_id: str, server: str
    ) -> tuple[Any, int]: ...

    async def update_chunithm_binding(
        self, platform: InstantMessengerPlatform, im_id: str, server: str
    ) -> tuple[Any, int]: ...

    async def delete_chunithm_binding(
        self, platform: InstantMessengerPlatform, im_id: str, server: str, aime_id: int
    ) -> tuple[Any, int]: ...

    # -------------------------------------
    # ---------chunithm music apis---------
    async def get_all_chunithm_music(self) -> tuple[Any, int]: ...

    async def get_chunithm_music_difficulty_info(self, music_id: int) -> tuple[Any, int]: ...

    async def get_chunithm_music_basic_info(self, music_id: int) -> tuple[Any, int]: ...

    async def get_chunithm_chart_data(self, music_id: int) -> tuple[Any, int]: ...

    async def query_chunithm_music_data_batch(self, music_ids: List[int]) -> tuple[Any, int]: ...
    # -------------------------------------
    # ------chunithm music alias apis------
    async def get_chunithm_music_id_by_alias(self, alias: str) -> tuple[Any, int]: ...

    async def get_chunithm_music_all_aliases(self, music_id: int) -> tuple[Any, int]: ...

    async def add_chunithm_music_alias(self, music_id: int, alias: str) -> tuple[Any, int]: ...

    async def delete_chunithm_music_alias(self, music_id: int) -> tuple[Any, int]: ...
    # -------------------------------------
    # -------pjsk suite data api-----------
    async def get_pjsk_suite_data(
        self,
        user_id: int,
        server: SekaiBindingServerRegion = SekaiBindingServerRegion.jp,
        data_type: SekaiSuiteDataType = SekaiSuiteDataType.suite,
    ) -> tuple[Any, int]:
        return await self.call_api(path=f"/private/{str(server)}/{str(data_type)}/{user_id}")

    async def update_pjsk_suite_data_policy(
        self,
        user_id: int,
        server: SekaiBindingServerRegion = SekaiBindingServerRegion.jp,
        data_type: SekaiSuiteDataType = SekaiSuiteDataType.suite,
        **configs,
    ) -> tuple[Any, int]: ...
