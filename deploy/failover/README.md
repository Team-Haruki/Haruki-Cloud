# Haruki Failover Runbook

These scripts codify the HA plan without making failover automatic. Keep the
single writer decision manual: a node may accept writes only after its business
databases have been restored from the current backup source and
`HARUKI_NODE_READ_ONLY=false` is set intentionally.

## Data split

- `haruki_sekai` is masterdata DB state. Secondary Cloud should keep using the
  existing `sekai.remote_sync` / secondary masterdata sync path.
- Business/user data is backed up separately:
  - `haruki_bot`
  - `haruki_pjsk`
  - `haruki_users`
- Add `haruki_censor` to `HARUKI_BUSINESS_DB_DBS` only if censor history must be
  included in the failover authority set.

## Normal backup node loop

Install `dump-business-dbs-from-active.sh` and
`lib/haruki-failover-common.sh` on the backup node under:

```bash
/data/HarukiServices/scripts/failover/
```

Install the timer files from `systemd/` into `/etc/systemd/system/`, then run:

```bash
systemctl daemon-reload
systemctl enable --now haruki-business-db-backup.timer
```

Defaults pull from the primary node as `root` with the configured SSH key:

```bash
HARUKI_ACTIVE_DATA_SSH_HOST=yamamoto.j8.network
HARUKI_ACTIVE_DATA_SSH_PORT=60022
HARUKI_ACTIVE_DATA_SSH_USER=root
HARUKI_ACTIVE_DATA_SSH_KEY=/root/.ssh/haruki_active_data_ed25519
```

## Secondary standby dump loop

The secondary Cloud node can read the active business database directly while
it is kept read-only. It should also keep local business DB dumps current, but
it must not restore them automatically while the primary is active. This keeps
the secondary ready for manual failover without creating a second writer or
dropping databases under a running Cloud process.

Normal secondary Cloud database routing is provided by:

```bash
/data/HarukiServices/configs/docker-compose.vm100.business-db-source.yml
```

The overlay consumes full DSNs from:

```text
HARUKI_BUSINESS_BOT_DB_URL
HARUKI_BUSINESS_PJSK_DB_URL
HARUKI_BUSINESS_USERS_DB_URL
```

In normal operation these point to the active primary PostgreSQL. During manual
failover, repoint them to the restored Shenzhen backup database or the
secondary's restored local PostgreSQL, then recreate `haruki-cloud`.

Install these files on the secondary:

```bash
/data/HarukiServices/scripts/sync-business-db-dumps-from-primary.sh
/etc/systemd/system/haruki-business-db-dump-sync.service
/etc/systemd/system/haruki-business-db-dump-sync.timer
```

Then enable the timer:

```bash
systemctl daemon-reload
systemctl enable --now haruki-business-db-dump-sync.timer
systemctl start haruki-business-db-dump-sync.service
```

The default dump set is:

```text
haruki_bot,haruki_pjsk,haruki_users
```

These dumps are stored under:

```text
/data/HarukiServices/data/business-db-dumps
```

## Primary failure

1. Confirm the primary is not accepting writes.
2. If the primary is reachable but should not restart, run on the primary:

```bash
HARUKI_FENCE_CONFIRM=FENCE_PRIMARY \
  /data/HarukiServices/scripts/failover/fence-local-primary.sh
```

3. On the secondary Cloud node, pull the backup dumps:

```bash
/data/HarukiServices/scripts/failover/sync-business-db-dumps-from-backup.sh
```

4. Restore local business DBs on the secondary:

```bash
HARUKI_COMPOSE_PROFILES=standby-cloud \
HARUKI_RESTORE_CONFIRM=RESTORE_BUSINESS_DBS \
  /data/HarukiServices/scripts/failover/restore-business-db-dumps-local.sh
```

5. If writing to the secondary's restored local PostgreSQL, set the business
   DSNs back to the local Postgres host in `.env`; if writing to a live
   restored Shenzhen backup PostgreSQL, point the same DSNs to that host.

6. Start the secondary Cloud as the temporary writer:

```bash
HARUKI_COMPOSE_PROFILES=standby-cloud \
HARUKI_UNFENCE_SERVICES=postgres,redis,haruki-cloud,haruki-caddy,haruki-drawing \
HARUKI_UNFENCE_CONFIRM=RESTORED_FROM_BACKUP \
  /data/HarukiServices/scripts/failover/unfence-local-node-after-restore.sh
```

7. Recreate `haruki-cloud` with the secondary overlays, then update the EdgeOne
   static routing JSON so clients choose the secondary API.

## Primary recovery

Do not allow the primary PostgreSQL/Cloud containers to auto-start before the
primary has been restored from the backup authority.

1. Keep or apply the fence on the recovered primary:

```bash
HARUKI_FENCE_CONFIRM=FENCE_PRIMARY \
  /data/HarukiServices/scripts/failover/fence-local-primary.sh
```

2. Point the backup job at the current temporary writer until the latest
   secondary data is captured:

```bash
HARUKI_ACTIVE_DATA_SSH_HOST=100.126.23.61
HARUKI_ACTIVE_DATA_SSH_PORT=40022
HARUKI_ACTIVE_DATA_SSH_USER=root
```

3. Pull backup dumps onto the primary and restore:

```bash
/data/HarukiServices/scripts/failover/sync-business-db-dumps-from-backup.sh

HARUKI_RESTORE_CONFIRM=RESTORE_BUSINESS_DBS \
  /data/HarukiServices/scripts/failover/restore-business-db-dumps-local.sh
```

4. Reopen writes on the primary:

```bash
HARUKI_UNFENCE_CONFIRM=RESTORED_FROM_BACKUP \
  /data/HarukiServices/scripts/failover/unfence-local-node-after-restore.sh
```

5. Put the secondary back into standby/read-only mode and update EdgeOne JSON
   back to primary.
