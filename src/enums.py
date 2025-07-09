from enum import Enum


class RequestDatabaseType(str, Enum):
    main = "main"
    suite = "suite"

    def __str__(self) -> str:
        return self.value


class SekaiBindingServerRegion(str, Enum):
    jp = "jp"
    en = "en"
    tw = "tw"
    kr = "kr"
    cn = "cn"
    default = "default"

    def __str__(self):
        return self.value


class SekaiSuiteDataType(str, Enum):
    suite = "suite"
    mysekai = "mysekai"

    def __str__(self) -> str:
        return self.value


class InstantMessengerPlatform(str, Enum):
    qq = "qq"
    qq_bot = "qq_bot"
    discord = "discord"
    telegram = "telegram"

    def __str__(self):
        return self.value


class PjskAliasType(str, Enum):
    music = "music"
    character = "character"

    def __str__(self):
        return self.value
