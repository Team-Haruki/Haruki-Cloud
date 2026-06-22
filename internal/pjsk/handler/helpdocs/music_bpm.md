# 查歌曲 BPM

## 指令
- `/查BPM`
- `/查bpm`
- `/pjsk bpm`

## 用法
- `/查BPM 歌曲名或歌曲ID`
- `/查BPM 歌曲名 难度`
- `/查BPM music123 master`

## 参数
- 歌曲可写正式曲名、歌曲别名、`music123`、纯数字 ID。
- 难度可写 `easy` `normal` `hard` `expert` `master` `append` 或缩写。
- 歌名里包含 `help` 时会按普通歌名查询；只有单独发送 `-help` 或 `-h` 才进入帮助。
- 如果别名匹配到多个歌曲，会返回候选列表，请改用歌曲 ID 查询。

## 示例
- `/查BPM Help me, ERINNNNNN!!`
- `/查BPM Help me, ERINNNNNN!! master`
- `/查BPM music123 expert`
