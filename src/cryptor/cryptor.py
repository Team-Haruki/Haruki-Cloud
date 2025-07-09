import os
import msgpack

from cryptography.hazmat.backends import default_backend
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes


class HarukiCloudCryptor(object):
    def __init__(self, key):
        self._key = key

    async def pack(self, content):
        nonce = os.urandom(12)
        cipher = Cipher(algorithms.AES(self._key), modes.GCM(nonce), backend=default_backend())
        encryptor = cipher.encryptor()
        data = msgpack.packb(content, use_single_float=True)
        ciphertext = encryptor.update(data) + encryptor.finalize()
        result = nonce + encryptor.tag + ciphertext
        return result

    async def unpack(self, content):
        nonce = content[:12]
        tag = content[12:28]
        ciphertext = content[28:]
        cipher = Cipher(algorithms.AES(self._key), modes.GCM(nonce, tag), backend=default_backend())
        decryptor = cipher.decryptor()
        decrypted_data = decryptor.update(ciphertext) + decryptor.finalize()
        return msgpack.unpackb(decrypted_data, strict_map_key=False)
