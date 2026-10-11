# 逮捕难度

设置查询自己绑定账号时，`/逮捕` 统计哪些难度。默认开启 EXPERT、MASTER。

## 用法
- `/逮捕难度 <难度> <开启|关闭> …`

## 参数
- `难度`：`easy` `normal` `hard` `expert` `master` `append`，也可以写 `hd` `ex` `apd`
- 开启状态：`开启`、`开`、`on`、`enable`、`enabled`、`true`、`1`
- 关闭状态：`关闭`、`关`、`off`、`disable`、`disabled`、`false`、`0`
- 紧凑写法：中文开关可以直接接在难度后面，例如 `append开`、`easy关闭`
- 一次可以设置多组，用空格、逗号、顿号或换行分隔；没有写到的难度保持原设置

## 示例
- `/逮捕难度 append 开启`
- `/逮捕难度 easy关 normal关`

## 说明
- 设置对你的所有绑定账号生效，不需要写 `u序号`
- 通过 `@群友` 或直接写游戏 UID 查询时，仍统计 EXPERT、MASTER
