import os
import msgpack
import asyncio
from typing import Union

from cryptography.hazmat.backends import default_backend
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes


class HarukiCryptor:
    def __init__(self, key):
        self._key = key

    def _pack_sync(self, content: Union[dict, list]) -> bytes:
        nonce = os.urandom(12)
        cipher = Cipher(algorithms.AES(self._key), modes.GCM(nonce), backend=default_backend())
        encryptor = cipher.encryptor()
        data = msgpack.packb(content, use_single_float=True)
        ciphertext = encryptor.update(data) + encryptor.finalize()
        result = nonce + encryptor.tag + ciphertext
        return result

    def _unpack_sync(self, content: bytes) -> Union[dict, list]:
        nonce = content[:12]
        tag = content[12:28]
        ciphertext = content[28:]
        cipher = Cipher(algorithms.AES(self._key), modes.GCM(nonce, tag), backend=default_backend())
        decryptor = cipher.decryptor()
        decrypted_data = decryptor.update(ciphertext) + decryptor.finalize()
        return msgpack.unpackb(decrypted_data, strict_map_key=False)

    async def pack(self, content: Union[dict, list]) -> bytes:
        return await asyncio.to_thread(self._pack_sync, content)

    async def unpack(self, content: bytes) -> Union[dict, list]:
        return await asyncio.to_thread(self._unpack_sync, content)
