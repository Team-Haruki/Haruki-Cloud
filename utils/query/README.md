# Query Toolkit Mapping

The unregistered private query APIs were consolidated into `utils/query`.

## Chunithm

- `GET /chunithm/user/:haruki_user_id/default` -> `Client.GetChunithmDefaultServer(ctx, harukiUserID)`
- `GET /chunithm/user/:haruki_user_id/:server` -> `Client.GetChunithmBinding(ctx, harukiUserID, server)`

## PJSK

- `GET /pjsk/user/:haruki_user_id/binding` -> `Client.GetPJSKBindings(ctx, harukiUserID, server)`
- `GET /pjsk/user/:haruki_user_id/binding/default` -> `Client.GetPJSKDefaultBinding(ctx, harukiUserID, server)`
- `GET /pjsk/user/:haruki_user_id/preference` -> `Client.GetPJSKPreferences(ctx, harukiUserID)`
- `GET /pjsk/user/:haruki_user_id/preference/:option` -> `Client.GetPJSKPreference(ctx, harukiUserID, option)`

## Users

- `GET /users?platform=...&user_id=...` -> `Client.GetUserByPlatform(ctx, platform, platformUserID)`
- `GET /users/:haruki_user_id` -> `Client.GetUserByID(ctx, harukiUserID)`

