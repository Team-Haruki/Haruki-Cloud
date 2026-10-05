"""Offline v1 -> v2 housing snapshot migration. Stop all snapshot writers first.

Usage: python3 migrate.py old.json new.json
The original is never modified. Keep it to roll back with the old Cloud image.
"""
import copy
import hashlib
import json
import re
import sys
from pathlib import Path

ORIGINS = {
    "cn": "https://mk-prod-tos.tos-cn-shanghai.volces.com",
    "tw": "https://mkoversea-prod-bucket.s3.ap-northeast-1.amazonaws.com",
    "kr": "https://mkkorea-prod-bucket.s3.ap-northeast-1.amazonaws.com",
}
PREFIX = "/image/mysekai-housing-competition/thumbnail/"
PATH = re.compile(r"[a-f0-9]{64}/[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}")


def key(entry):
    parts = [str(entry.get("competition_id", 0)), str(entry.get("submitted_at", 0)),
             entry.get("thumbnail_path", "").strip(), entry.get("entry_name", "").strip()]
    return hashlib.sha256("\0".join(parts).encode()).hexdigest()


def migrate(snapshot):
    if snapshot.get("version") != 1:
        raise ValueError("Expected a v1 snapshot")
    result = copy.deepcopy(snapshot)
    changed = 0
    for bucket in result["buckets"]:
        region = bucket["key"]["region"]
        keys = set()
        for entry in bucket["entries"]:
            path = entry.get("thumbnail_path", "").strip()
            if "://" in path:
                prefix = ORIGINS.get(region, "") + PREFIX
                if region not in ORIGINS or not path.startswith(prefix):
                    raise ValueError("Unexpected thumbnail origin")
                path = path[len(prefix):]
                if not PATH.fullmatch(path):
                    raise ValueError("Invalid thumbnail object path")
                entry["thumbnail_path"] = path
                changed += 1
            entry["cache_key"] = key(entry)
            if entry["cache_key"] in keys:
                raise ValueError("Duplicate upload in snapshot; inspect before migration")
            keys.add(entry["cache_key"])
    result["version"] = 2
    return result, changed


if __name__ == "__main__":
    source, target = map(Path, sys.argv[1:])
    result, changed = migrate(json.loads(source.read_text()))
    with target.open("x") as output:
        json.dump(result, output, ensure_ascii=False, separators=(",", ":"))
    print(json.dumps({"buckets": len(result["buckets"]), "entries": sum(len(b["entries"]) for b in result["buckets"]), "paths_migrated": changed}))
