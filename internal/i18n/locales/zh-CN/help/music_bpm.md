# 查 BPM

## 用法
- `/查BPM <歌曲名|别名|歌曲 ID> [难度]`

## 参数
- `歌曲名|别名|歌曲 ID`：必填；歌曲 ID 可写 `music123`。
- 难度：可选，`expert` `master` 等；用于指定谱面 BPM 信息。
- 区服前缀：可在指令前加 `jp` `cn` `en` `tw` `kr`，例如 `/jp查曲`；不写时使用默认区服。

## 输出
- 返回歌曲 BPM；如果匹配到多首歌，会返回候选并提示用 ID 查询。

## 示例
- `/查BPM Help me, ERINNNNNN!!`
- `/查BPM music123 master`
