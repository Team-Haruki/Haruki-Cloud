# Bot 指令端到端测试清单

> 本文档列出 Haruki-Cloud PJSK Bot API 所有已注册指令，供端到端集成测试使用。
>
> **Bot API 路径格式**：`POST /api/v2/bot/:botId/pjsk/<path>`  
> **区域前缀**：每条路径均支持 `jp/`、`tw/`、`kr/`、`en/` 前缀（如 `jp/card/detail`），无前缀时默认为 JP 区。  
> **状态**：✅ 已启用 | ⚠️ 需 Toolbox 绑定 | 🚫 Disabled（代码中 `Disabled: true`）

---

## 1. 卡牌 (card)

| 路径 | 代表指令 | 全部触发词 | 状态 | 备注 |
|------|---------|-----------|------|------|
| `card/detail` | `/查卡` | `/card-detail` `/查卡` `/查牌` `/查卡牌` `/pjsk card` | ✅ | 按名称/ID/属性查单张卡 |
| `card/list` | `/卡牌列表` | `/卡牌列表` `/cards` `/pjsk cards` `/card-list` | ✅ | 卡牌筛选列表 |
| `card/box` | `/查箱` | `/查箱` `/卡牌一览` `/卡面一览` `/卡一览` `/box` `/card-box` `/pjsk box` | ✅ | 玩家卡牌一览，需用户绑定 |
| `card/image` | `/查卡面` | `/pjsk card img` `/查卡面` `/卡面原图` `/卡面` `/card` `/卡图` | ✅ | 查卡面原图 |
| `card/story` | `/卡牌剧情` | `/pjsk card story` `/卡牌剧情` `/卡面剧情` `/卡剧情` `/卡牌故事` `/卡面故事` `/卡故事` | 🚫 | 未实现，仅 JP |

---

## 2. 音乐/乐曲 (music)

| 路径 | 代表指令 | 全部触发词 | 状态 | 备注 |
|------|---------|-----------|------|------|
| `music` | `/查曲` | `/查曲` `/查歌` `/查乐` `/查音乐` `/查询乐曲` `/查歌曲` `/歌曲` `/乐曲` `/song` `/music` | ✅ | 查单曲信息 |
| `music/list` | `/歌曲列表` | `/歌曲列表` `/歌曲一览` `/乐曲列表` `/乐曲一览` `/难度排行` `/定数表` `/歌曲定数` `/查乐曲` `/music-list` `/pjsk music list` | ✅ | 难度/定数表 |
| `music/chart` | `/谱面` | `/pjsk chart` `/谱面查询` `/铺面查询` `/谱面预览` `/铺面预览` `/谱面` `/铺面` `/查谱面` `/查铺面` `/查谱` `/技能预览` | ✅ | 谱面预览 |
| `music/rewards` | `/曲目奖励` | `/曲目奖励` `/歌曲奖励` `/music rewards` `/music-rewards` `/pjsk music rewards` `/打歌奖励` `/歌曲挖矿` `/打歌挖矿` | ✅ | 打歌奖励/挖矿 |
| `music/progress` | `/打歌进度` | `/打歌进度` `/歌曲进度` `/打歌信息` `/pjsk进度` `/progress` `/music-progress` `/pjsk music progress` `/pjsk progress` | ✅ ⚠️ | 需用户绑定 |
| `music/note-count` | `/物量` | `/pjsk note num` `/pjsk note count` `/物量` `/查物量` | ✅ | 谱面物量查询 |
| `music/bpm` | `/查bpm` | `/pjsk bpm` `/查bpm` `/查BPM` | ✅ | 查曲 BPM |
| `music/cover` | `/查曲绘` | `/pjsk music cover` `/查曲绘` `/曲绘` | ✅ | 查曲绘原图 |

---

## 3. 组卡/队伍 (deck)

| 路径 | 代表指令 | 全部触发词 | 状态 | 备注 |
|------|---------|-----------|------|------|
| `deck/event` | `/活动组卡` | `/pjsk event card` `/pjsk event deck` `/pjsk deck` `/活动组卡` `/活动组队` `/活动卡组` `/活动配队` `/组卡` `/组队` `/配队` `/指定属性组卡` `/模拟组卡` | ✅ ⚠️ | 活动组卡，需用户绑定 |
| `deck/challenge` | `/挑战组卡` | `/pjsk challenge card` `/pjsk challenge deck` `/挑战组卡` `/挑战组队` `/挑战卡组` `/挑战配队` | ✅ ⚠️ | 挑战赛组卡 |
| `deck/no-event` | `/长草组卡` | `/pjsk no event deck` `/pjsk best deck` `/长草组卡` `/长草组队` `/最强卡组` `/最强组卡` | ✅ ⚠️ | 非活动期最强卡组 |
| `deck/bonus` | `/加成组卡` | `/pjsk bonus deck` `/pjsk bonus card` `/加成组卡` `/加成组队` `/控分组卡` `/控分配队` | ✅ ⚠️ | 加成/控分组卡 |
| `deck/mysekai` | `/烤森组卡` | `/mysekai deck` `/pjsk mysekai deck` `/烤森组卡` `/烤森组队` `/ms组卡` `/ms组队` | ✅ ⚠️ | MySekai 场景组卡 |
| *(score-up)* | `/实效` | `/实效` `/倍率` `/时效` `/pjsk score up` | 🚫 | 未实现 |

---

## 4. 活动 (event)

| 路径 | 代表指令 | 全部触发词 | 状态 | 备注 |
|------|---------|-----------|------|------|
| `event/list` | `/活动列表` | `/pjsk events` `/events` `/活动列表` `/活动一览` `/event-list` | ✅ | 活动列表 |
| `event` | `/查活动` | `/pjsk event` `/活动` `/查活动` `/event` | ✅ | 当前/指定活动信息 |
| `event/record` | `/活动记录` | `/pjsk event record` `/活动记录` `/冲榜记录` | ✅ ⚠️ | 需用户绑定 |
| `event/story` | `/活动剧情` | `/pjsk event story` `/活动剧情` `/活动故事` `/活动总结` | 🚫 | 未实现，仅 JP |

---

## 5. 榜线/SK (sk)

| 路径 | 代表指令 | 全部触发词 | 状态 | 备注 |
|------|---------|-----------|------|------|
| `sk/line` | `/sk线` | `/sk-line` `/sk线` `/榜线` `/pjsk sk line` `/skl` | ✅ | 当前榜线 |
| `sk/query` | `/sk` | `/sk-query` `/sk查询` `/sk查分` `/pjsk sk board` `/pjsk board` `/sk` | ✅ | 查指定分数榜位 |
| `sk/speed` | `/时速` | `/pjsk sk speed` `/时速` `/sks` `/skv` `/sk时速` `/sk-speed` | ✅ | 当前榜线时速 |
| `sk/speed` | `/日速` | `/pjsk sk daily speed` `/日速` `/skds` `/skdv` `/sk日速` | ✅ | 日均速度 |
| `sk/check-room` | `/查房` | `/sk-check-room` `/sk查房` `/查房` `/cf` `/pjsk查房` `/csb` `/冲水板` | ✅ | 查 CS 房间/水表 |
| `sk/player-trace` | `/玩家轨迹` | `/sk-player-trace` `/玩家轨迹` `/ptr` `/pjsk玩家追踪` | ✅ ⚠️ | 需用户绑定 |
| `sk/rank-trace` | `/档线轨迹` | `/sk-rank-trace` `/档线轨迹` `/rtr` `/skt` `/sklt` `/pjsk追踪` | ✅ | 档线历史轨迹 |
| `sk/rank-trace` | `/sk预测` | `/pjsk sk predict` `/sk预测` `/榜线预测` `/skp` | ✅ | 榜线预测（共享路径） |
| `sk/winrate` | `/胜率预测` | `/pjsk winrate predict` `/胜率预测` `/5v5预测` `/胜率` `/预测胜率` | ✅ | 5v5 胜率 |

---

## 6. MySekai / 烤森

> ⚠️ **所有 MySekai 指令需用户绑定 Toolbox 账号**（suite + mysekai 数据），且对 `region=cn` 关闭（除非在 `allow_cn_mysekai` 白名单中）

| 路径 | 代表指令 | 全部触发词 | 状态 | 备注 |
|------|---------|-----------|------|------|
| `mysekai/resource` | `/烤森资源` | `/pjsk mysekai res` `/mysekai-resource` `/mysekai资源` `/烤森资源` `/msa` | ✅ ⚠️ | 资源采集状态 |
| `mysekai/map` | `/烤森地图` | `/pjsk mysekai map` `/mysekai-map` `/mysekai地图` `/烤森地图` `/msm` `/msmap` | ✅ ⚠️ | 地图/收获地图 |
| `mysekai/talk-list` | `/烤森对话列表` | `/mysekai-talk-list` `/mysekai对话列表` `/烤森对话列表` | ✅ ⚠️ | 角色对话列表 |
| `mysekai/fixture-list` | `/烤森家具列表` | `/mysekai-fixture-list` `/mysekai家具列表` `/烤森家具列表` | ✅ ⚠️ | 已获得家具列表 |
| `mysekai/fixture-detail` | `/msf` | `/pjsk mysekai furniture` `/pjsk mysekai fixture` `/msf` `/mysekai 家具` `/家具列表` | ✅ | 家具详情（不需 snapshot） |
| `mysekai/door-upgrade` | `/msg` | `/pjsk mysekai gate` `/mysekai-door-upgrade` `/mysekai大门升级` `/烤森大门升级` `/msg` `/msgate` | ✅ ⚠️ | 大门升级材料 |
| `mysekai/music-record` | `/烤森唱片` | `/pjsk mysekai musicrecord` `/mysekai-music-record` `/mysekai唱片` `/烤森唱片` `/mss` `/msr` `/mssong` | ✅ ⚠️ | 音乐唱片收集 |
| `mysekai/blueprint` | `/msb` | `/pjsk mysekai blueprint` `/mysekai blueprint` `/msb` `/mysekai 蓝图` | ✅ ⚠️ | 蓝图列表 |
| `mysekai/photo` | `/msp` | `/pjsk mysekai photo` `/pjsk mysekai picture` `/msp` `/mysekai 照片` | ✅ ⚠️ | 照片列表 |

---

## 7. 个人资料 / 绑定 (profile)

| 路径 | 代表指令 | 全部触发词 | 状态 | 参数 | 备注 |
|------|---------|-----------|------|------|------|
| `profile` | `/个人中心` | `/个人中心` `/profile` | ✅ ⚠️ | `[@用户]` `[游戏ID]` `[u序号]` | 个人信息渲染；支持 u[i] 查指定绑定 |
| `profile/bind` | `/绑定` | `/pjsk bind` `/pjsk id` `/绑定` `/pjsk 绑定` | ✅ | `<账号ID>` | 绑定 PJSK 账号 |
| `profile/bind/list` | `/绑定列表` | `/绑定列表` `/pjsk bind list` `/pjsk绑定列表` | ✅ | 无 | 列出所有已绑定账号 |
| `profile/unbind` | `/解绑` | `/pjsk unbind` `/解绑` `/取消绑定` | ✅ | `<账号ID \| u序号>` | 解除绑定 |
| `profile/default` | `/设置主账号` | `/pjsk set main` `/pjsk主账号` `/设置主账号` `/设置默认绑定` | ✅ | `<账号ID \| u序号>` | 设置默认绑定；带区域前缀设置区服默认 |
| `profile/default/clear` | `/清除默认绑定` | `/取消默认绑定` `/清除默认绑定` `/取消主账号` `/清除主账号` | ✅ | `[账号ID \| u序号]` | 无参数时清除当前scope默认；带区域前缀指定区服 |
| `profile/suite/hide` | `/隐藏抓包` | `/pjsk hide suite` `/pjsk隐藏抓包` `/隐藏抓包` | ✅ | `[u序号]` | 隐藏 suite 数据 |
| `profile/suite/show` | `/展示抓包` | `/pjsk show suite` `/展示抓包` | ✅ | `[u序号]` | 展示 suite 数据 |
| `profile/mysekai/hide` | `/隐藏烤森抓包` | `/pjsk hide mysekai` `/隐藏烤森抓包` | ✅ | `[u序号]` | 隐藏 MySekai 数据 |
| `profile/mysekai/show` | `/展示烤森抓包` | `/pjsk show mysekai` `/展示烤森抓包` | ✅ | `[u序号]` | 展示 MySekai 数据 |
| `profile/visibility/hide` | `/隐藏ID` | `/pjsk hide id` `/隐藏id` `/隐藏ID` | ✅ | `[u序号]` | 隐藏 UID |
| `profile/visibility/show` | `/显示ID` | `/pjsk show id` `/显示id` `/展示ID` | ✅ | `[u序号]` | 显示 UID |
| `profile/check-data` | `/抓包数据` | `/pjsk check data` `/抓包数据` `/抓包状态` `/抓包信息` `/sud` | ✅ | `[u序号]` | 查 Suite 抓包更新时间（仅自己）；输出含时区 |
| `profile/check-data-mysekai` | `/msd` | `/msd` `/pjsk check mysekai data` `/烤森抓包` `/烤森抓包数据` | ✅ | `[u序号]` | 查 MySekai 抓包更新时间（仅自己）；输出含时区 |
| `profile/verify` | `/pjsk verify` | `/pjsk verify` `/pjsk验证` | ✅ | `[u序号]` | 账号验证；支持 u[i] 指定绑定 |
| `profile/verify/list` | `/pjsk verify list` | `/pjsk verify list` `/pjsk验证列表` `/pjsk验证状态` | ✅ | 无 | 列出已验证账号；无前缀用全局默认绑定的区服 |
| `profile/bg/upload` | `/上传个人信息背景` | `/pjsk upload profile bg` `/上传个人信息背景` `/上传个人背景` | ✅ | `[图片]` | 上传自定义背景图 |
| `profile/bg/clear` | `/清空个人信息背景` | `/pjsk clear profile bg` `/清空个人信息背景` | ✅ | 无 | 清除背景图 |
| `profile/bg/adjust` | `/调整个人信息背景` | `/pjsk adjust profile` `/调整个人信息背景` `/设置个人信息` | ✅ | `[横屏\|竖屏] [模糊 0~10] [透明 0~100]` | 调整背景图模糊/透明度 |
| `profile/reg-time` | `/注册时间` | `/注册时间` `/pjsk reg time` `/查时间` | ✅ | `[u序号]` | 查自己账号注册时间；仅自己 + u[i]；输出含时区（默认 UTC+8）|

---

## 8. 教育/成长系统 (education)

| 路径 | 代表指令 | 全部触发词 | 状态 | 备注 |
|------|---------|-----------|------|------|
| `education/challenge` | `/挑战信息` | `/pjsk challenge info` `/挑战信息` `/挑战详情` `/挑战进度` `/每日挑战` | ✅ ⚠️ | 每日挑战进度 |
| `education/power` | `/加成信息` | `/pjsk power bonus info` `/加成信息` `/加成详情` `/加成进度` `/角色加成` | ✅ ⚠️ | 角色加成信息 |
| `education/area` | `/区域道具` | `/pjsk area item` `/area item` `/区域道具` `/区域道具升级` | ✅ ⚠️ | 区域道具升级材料 |
| `education/bonds` | `/羁绊` | `/pjsk bonds` `/羁绊` `/羁绊等级` `/角色羁绊` `/牵绊` | ✅ ⚠️ | 角色羁绊等级 |
| `education/leader` | `/队长统计` | `/队长统计` `/领队统计` `/角色领队` `/pjsk leader count` | ✅ ⚠️ | 队长使用次数统计 |

---

## 9. 分数/控分 (score)

| 路径 | 代表指令 | 全部触发词 | 状态 | 备注 |
|------|---------|-----------|------|------|
| `score` | `/控分` | `/分数` `/查分数` `/pjsk score` `/score control` `/控分` | ✅ | 控分计算 |
| `score/custom-room` | `/自定义控分` | `/pjsk custom room score` `/自定义房间控分` `/自定义房控分` `/自定义控分` | ✅ | 自定义房间控分 |
| `score/music-meta` | `/歌曲meta` | `/pjsk music meta` `/music meta` `/歌曲meta` `/曲目meta` | ✅ | 歌曲 meta 信息 |
| `score/music-board` | `/歌曲排行` | `/pjsk music board` `/music board` `/歌曲排行` `/歌曲比较` `/曲目榜` | ✅ | 歌曲分数排行 |

---

## 10. 别名管理 (alias)

| 路径 | 代表指令 | 全部触发词 | 状态 | 备注 |
|------|---------|-----------|------|------|
| `alias` | `/歌曲别名` | `/pjsk alias` `/music alias` `/歌曲别名` `/查歌曲别名` | ✅ | 查曲目别名 |
| `alias/add` | `/添加歌曲别名` | `/music alias add` `/pjsk alias add` `/添加歌曲别名` | ✅ | 添加曲目别名（需审核） |
| `alias/del` | `/删除歌曲别名` | `/music alias del` `/pjsk alias del` `/删除歌曲别名` | ✅ | 删除曲目别名（需权限） |
| `chara alias` | `/角色别名` | `/pjsk chara alias` `/chara alias` `/角色别名` `/查角色别名` | ✅ | 查角色别名 |
| `chara alias/add` | `/添加角色别名` | `/pjsk chara alias add` `/chara alias add` `/添加角色别名` | ✅ | 添加角色别名 |
| `chara alias/del` | `/删除角色别名` | `/pjsk chara alias del` `/chara alias del` `/删除角色别名` | ✅ | 删除角色别名 |
| `alias/pending` | `/待审核别名` | `/待审核别名` `/别名待审核` `/歌曲别名待审核` `/角色别名待审核` | ✅ | 管理员查待审核 |
| `alias/approve` | `/同意别名` | `/同意别名` `/通过别名` | ✅ | 管理员审核通过 |
| `alias/reject` | `/拒绝别名` | `/拒绝别名` | ✅ | 管理员拒绝 |

---

## 11. 杂项 (misc)

| 路径 | 代表指令 | 全部触发词 | 状态 | 备注 |
|------|---------|-----------|------|------|
| `misc/birthday` | `/生日` | `/pjsk chara birthday` `/角色生日` `/生日` `/查生日` | ✅ | 角色生日查询 |
| `stamp` | `/贴纸` | `/贴纸` `/查贴纸` `/pjsk贴纸` `/pjsk表情` `/pjsk stamp` `/stamp` | ✅ | 贴纸查询 |
| `vlive` | `/虚拟live` | `/pjsk live` `/虚拟live` `/pjsk vlive` `/vlive` | ✅ | 虚拟 Live 信息 |
| `arrest` | `/逮捕` | `/逮捕` `/pjsk逮捕` `/pjsk arrest` | ✅ | 逮捕功能；支持 `[@用户]` `[游戏ID]` `[u序号]` |
| `gacha` | `/卡池` | `/pjsk gacha` `/卡池列表` `/卡池一览` `/卡池` `/查卡池` | ✅ | 卡池列表 |
| *(gacha/record)* | `/抽卡记录` | `/pjsk gacha record` `/抽卡记录` `/抽卡历史` | 🚫 | 未实现 |
| *(help)* | `/帮助` | `/help` `/帮助` | 🚫 | 未实现 |
| *(update)* | `/pjsk更新` | `/pjsk update` `/pjsk refresh` `/pjsk更新` | 🚫 | 未实现 |
| *(ng word)* | `/pjsk屏蔽词` | `/pjsk ng` `/pjsk ngword` | 🚫 | 未实现 |

---

## 12. 区域支持说明

大部分指令支持区域前缀，使用方式：
- 无前缀 → 使用用户的全局默认绑定区服（无默认绑定时 fallback 为 JP）
- `/jp查曲` → JP 区
- `/tw查曲` → TW 区  
- `/kr查曲` → KR 区
- `/en查曲` → EN 区（WW）

Bot API 路径同理：`/api/v2/bot/:botId/pjsk/jp/music`

**例外（仅 JP）**：`card/story`、`event/story`、部分 MySekai 功能。

## 13. u[i] 绑定选择器说明

多绑定用户可通过 `u[i]` 参数指定操作哪个绑定账号（i 为绑定列表序号）。

支持 u[i] 的指令：
- **查询类**：`/个人中心 u1`（+ @用户/游戏ID）、`/逮捕 u2`（+ @用户/游戏ID）、`/注册时间 u1`（仅自己 + u[i]）
- **抓包查询**：`/sud u1`、`/msd u2`（仅自己 + u[i]）
- **验证**：`/pjsk verify u1`（仅自己 + u[i]）
- **设置类**：`/隐藏id u1`、`/显示id u2`、`/隐藏抓包 u1`、`/展示抓包 u2`、`/隐藏烤森抓包 u1`、`/展示烤森抓包 u2`
- **清除默认**：`/清除默认绑定`（无参数清除全局/区域默认）、`/清除默认绑定 u1`（指定账号）

不支持 u[i] 的指令：
- `/绑定`（直接提供游戏ID）
- `/解绑`（u序号或游戏ID）
- `/设置主账号`（u序号或游戏ID）
- `/sud`、`/msd`、`/注册时间`、`/pjsk verify` 不支持 @他人和直接游戏ID（仅自己 + u[i]）

时区说明：`/注册时间`、`/sud`、`/msd` 输出时间默认使用 **UTC+8**，用户可在 UserSettings 中通过 `TimeZoneOffset`（如 `+09:00`）自定义时区。

---

## 测试状态追踪

| 功能组 | 已测试   | 通过 | 失败 | 备注                       | 测试人   |
|--------|-------|----|----|--------------------------|-------|
| card |       |    |    |                          |       |
| music |       |    |    |                          |       |
| deck |       |    |    |                          |       |
| event |       |    |    |                          |       |
| sk |       |    |    |                          |       |
| mysekai | 9/9   | 9  | 0  | 对话列表/唱片列表/门 数据解析存在问题需要修复 | 锡纸    |
| profile | 18/20 | 17 | 1  | bg相关需要处理client实现         | 锡纸    |
| education |       |    |    |                          |       |
| score | 4/4   | 4  | 0  |                          | LQ、锡纸 |
| alias |       |    |    |                          |       |
| misc/gacha/stamp/vlive |       |    |    |                          |       |

---

*生成于 2026-03-28，基于 `test` 分支代码分析。*
