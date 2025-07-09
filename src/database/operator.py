from typing import Any, Tuple, List

from src.enums import SekaiBindingServerRegion, InstantMessengerPlatform, PjskAliasType, SekaiSuiteDataType

from .client import HarukiDBClient


class HarukiDBOperator(HarukiDBClient):
    # ---------pjsk binding apis---------
    async def get_pjsk_binding(
        self, platform: InstantMessengerPlatform, im_id: str, server: SekaiBindingServerRegion = None
    ) -> Tuple[Any, int]:
        data, status = await self.call_db_api(
            path=f"/pjsk/{platform}/user/{im_id}/binding", params={"server": server} if server else {}
        )
        return data, status

    async def add_pjsk_binding(
        self, platform: InstantMessengerPlatform, im_id: str, server: SekaiBindingServerRegion = None
    ) -> Tuple[Any, int]: ...

    async def get_pjsk_default_binding(
        self,
        platform: InstantMessengerPlatform,
        im_id: str,
        server: SekaiBindingServerRegion = SekaiBindingServerRegion.default,
    ) -> Tuple[Any, int]: ...

    async def set_pjsk_default_binding(
        self,
        platform: InstantMessengerPlatform,
        im_id: str,
        server: SekaiBindingServerRegion = SekaiBindingServerRegion.default,
    ) -> Tuple[Any, int]: ...

    async def delete_pjsk_default_binding(
        self,
        platform: InstantMessengerPlatform,
        im_id: str,
        server: SekaiBindingServerRegion = SekaiBindingServerRegion.default,
    ) -> Tuple[Any, int]: ...

    async def delete_pjsk_binding(
        self, platform: InstantMessengerPlatform, im_id: str, server: SekaiBindingServerRegion = None
    ) -> Tuple[Any, int]: ...

    async def update_pjsk_biding(
        self, platform: InstantMessengerPlatform, im_id: str, server: SekaiBindingServerRegion = None
    ) -> Tuple[Any, int]: ...

    # ------------------------------------
    # ---------pjsk alias apis------------
    async def query_object_id_by_alias(
        self, alias: str, alias_type: PjskAliasType = PjskAliasType.music, group_id: int = None
    ) -> Tuple[Any, int]: ...

    async def get_all_aliases(
        self, alias_id: int, alias_type: PjskAliasType = PjskAliasType.music, group_id: int = None
    ) -> Tuple[Any, int]: ...

    async def add_alias(
        self, alias_id: int, alias_type: PjskAliasType = PjskAliasType.music, group_id: int = None
    ) -> Tuple[Any, int]: ...

    async def delete_alias(
        self, alias_id: int, alias_type: PjskAliasType = PjskAliasType.music, group_id: int = None
    ) -> Tuple[Any, int]: ...

    async def get_pending_review_aliases(self) -> Tuple[Any, int]: ...

    async def approve_alias(self, pending_review_id: int) -> Tuple[Any, int]: ...

    async def reject_alias(self, pending_review_id: int, reason: str = None) -> Tuple[Any, int]: ...

    async def get_pending_review_alias_status(self, pending_review_id: int) -> Tuple[Any, int]: ...

    # -------------------------------------
    # ------pjsk user preferences apis-----

    async def get_all_preferences(self, platform: InstantMessengerPlatform, im_id: str) -> Tuple[Any, int]: ...

    async def get_specific_preference(
        self, platform: InstantMessengerPlatform, im_id: str, option: str
    ) -> Tuple[Any, int]: ...

    async def update_specific_preference(
        self, platform: InstantMessengerPlatform, im_id: str, option: str, value: str
    ) -> Tuple[Any, int]: ...

    async def delete_specific_preference(
        self, platform: InstantMessengerPlatform, im_id: str, option: str
    ) -> Tuple[Any, int]: ...

    # -------------------------------------
    # --------chunithm binding apis--------
    async def get_chunithm_default_server(self, platform: InstantMessengerPlatform, im_id: str) -> Tuple[Any, int]: ...

    async def set_chunithm_default_server(
        self, platform: InstantMessengerPlatform, im_id: str, server: str
    ) -> Tuple[Any, int]: ...

    async def delete_chunithm_default_server(
        self, platform: InstantMessengerPlatform, im_id: str, server: str
    ) -> Tuple[Any, int]: ...

    async def get_chunithm_binding(
        self, platform: InstantMessengerPlatform, im_id: str, server: str
    ) -> Tuple[Any, int]: ...

    async def update_chunithm_binding(
        self, platform: InstantMessengerPlatform, im_id: str, server: str
    ) -> Tuple[Any, int]: ...

    async def delete_chunithm_binding(
        self, platform: InstantMessengerPlatform, im_id: str, server: str, aime_id: int
    ) -> Tuple[Any, int]: ...

    # -------------------------------------
    # ---------chunithm music apis---------
    async def get_all_chunithm_music(self) -> Tuple[Any, int]: ...

    async def get_chunithm_music_difficulty_info(self, music_id: int) -> Tuple[Any, int]: ...

    async def get_chunithm_music_basic_info(self, music_id: int) -> Tuple[Any, int]: ...

    async def get_chunithm_chart_data(self, music_id: int) -> Tuple[Any, int]: ...

    async def query_chunithm_music_data_batch(self, music_ids: List[int]) -> Tuple[Any, int]: ...
    # -------------------------------------
    # ------chunithm music alias apis------
    async def get_chunithm_music_id_by_alias(self, alias: str) -> Tuple[Any, int]: ...

    async def get_chunithm_music_all_aliases(self, music_id: int) -> Tuple[Any, int]: ...

    async def add_chunithm_music_alias(self, music_id: int, alias: str) -> Tuple[Any, int]: ...

    async def delete_chunithm_music_alias(self, music_id: int) -> Tuple[Any, int]: ...
    # -------------------------------------
    # -------pjsk suite data api-----------
    async def get_pjsk_suite_data(
        self,
        user_id: int,
        server: SekaiBindingServerRegion = SekaiBindingServerRegion.jp,
        data_type: SekaiSuiteDataType = SekaiSuiteDataType.suite,
    ) -> Tuple[Any, int]:
        return await self.call_api(path=f"/private/{str(server)}/{str(data_type)}/{user_id}")

    async def update_pjsk_suite_data_policy(
        self,
        user_id: int,
        server: SekaiBindingServerRegion = SekaiBindingServerRegion.jp,
        data_type: SekaiSuiteDataType = SekaiSuiteDataType.suite,
        **configs,
    ) -> Tuple[Any, int]: ...
