# auto-chess

**无状态 AI 走子接口**：请求传入当前棋盘，响应只返回下一步最佳落子。

一个独立的多棋种 AI 走子服务。服务端不创建对局、不保存棋谱、不维护用户，也不返回新棋盘；棋盘和历史始终由调用方持有。

- 7 种棋类：中国象棋、国际象棋、五子棋、黑白棋、四子棋、斗兽棋、中国跳棋
- 国际象棋使用 Chess-3；中国象棋可选 godogpaw 或纯 Go NNUE（加载 Pikafish 2024 网络）
- 一套请求结构：FEN 棋种传 `fen`，网格棋种传扁平的 `board` 数组
- 无状态：`当前局面 → 下一步`
- 难度 / 思考时间 / 最大深度可调
- 支持 SSE 流式返回逐层思考过程，高难度时可观察实时进度
- 基于 [go-web-frame](https://github.com/chuccp/go-web-frame)（路由、过滤器、配置、生命周期）
- 内置 Go `html/template` 多页面站点：首页、接入指南、接口参考、SSE、棋种目录/详情、在线对弈、调试器
- `/play` 可直接与引擎对弈，对局自动保存在浏览器，支持继续、悔棋和切换棋种
- 对弈时可选择普通 `/api/v1/move` 或 SSE `/api/v1/move/stream` 接口；SSE 模式会显示深度、节点和耗时
- 已启用 CORS，浏览器前端可直接调用
- 默认亮色主题；导航可切换亮色 / 深色并记住选择
- 中文 / English 切换，页面、导航、棋种说明和接口文档均会切换语言

> 演示地址（本地服务启动后）：<http://127.0.0.1:8090/auto-chess/>

> 主要页面：`/auto-chess/` 首页 · `/auto-chess/guide` 接入指南 · `/auto-chess/docs` 接口参考 · `/auto-chess/docs/stream` SSE · `/auto-chess/games` 棋种 · `/auto-chess/play` 在线对弈 · `/auto-chess/playground` 调试器

> 这不是完整的对局平台：没有房间、账号、在线匹配或服务端会话；对弈记录仅保存在当前浏览器。

---

## 快速开始

要求 Go 1.26+。

```bash
go mod tidy
go run .
```

启动后：

- 首页：<http://127.0.0.1:8090/auto-chess/>
- API 文档：<http://127.0.0.1:8090/auto-chess/docs>
- 调试器：<http://127.0.0.1:8090/auto-chess/playground>
- 健康检查：<http://127.0.0.1:8090/auto-chess/api/v1/health>
- 棋种目录：<http://127.0.0.1:8090/auto-chess/api/v1/games>

端口和 `contextPath` 在 `application.yml` 中配置，也可以用环境变量覆盖：

```bash
PORT=9000 CONTEXT_PATH=games go run .
```

Windows PowerShell：

```powershell
$env:PORT=9000; $env:CONTEXT_PATH="games"; go run .
```

页面里的资源、导航和 API 调用都使用相对路径；修改 `contextPath` 后不需要改模板或 JS。默认值为 `auto-chess`。

### 多页面站点

文档站点由 Go `html/template` 服务端渲染，共用同一套布局、导航和静态资源：

| 路径 | 页面 |
|---|---|
| `/auto-chess/` | 首页 |
| `/auto-chess/guide` | 快速开始 / 接入指南 |
| `/auto-chess/docs` | 接口参考 |
| `/auto-chess/docs/stream` | SSE 流式思考 |
| `/auto-chess/games` | 棋种目录 |
| `/auto-chess/games/{game}` | 棋种详情：规则、坐标、初始局面、独立接口详解与示例 |
| `/auto-chess/play` | 在线对弈：人机轮流落子、本机保存记录 |
| `/auto-chess/playground` | 请求调试器（普通 + SSE） |

页面数据直接来自 `internal/registry`，模板与静态资源通过 `embed` 内置，不需要单独部署前端文件。布局为响应式，对弈棋盘、侧栏和导航在手机宽度下会自动重排和缩放。

主题默认亮色，顶部可切换深色；选择保存在浏览器。语言默认中文，顶部可切换 English；语言通过 `?lang=en` / `?lang=zh` 和 cookie 记忆，刷新或切换到其他页面后仍保持。每个棋种的详情页都有独立的“接口详解”：棋盘/走法格式、字段表、完整请求与响应、`apply-move`、`legal-moves`、SSE 示例和该棋种的注意事项。

---

## 核心接口

> 下文以 `/api/v1/...` 表示相对 `contextPath` 的 API 路径。默认 `contextPath: auto-chess`，实际地址为 `/auto-chess/api/v1/...`。

### `POST /api/v1/move`

请求当前局面，返回 AI 的下一步。

```json
{
  "game": "xiangqi",
  "fen": "rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1",
  "difficulty": "normal",
  "timeLimitMs": 1000
}
```

也可以把棋种放在路径里：

```
POST /api/v1/games/xiangqi/move
```

响应：

```json
{
  "ok": true,
  "data": {
    "game": "xiangqi",
    "engine": "godogpaw",
    "difficulty": "normal",
    "move": {
      "uci": "h2e2",
      "from": { "index": 25, "row": 2, "col": 7 },
      "to": { "index": 22, "row": 2, "col": 4 }
    },
    "gameOver": false,
    "status": { "state": "playing" },
    "evaluation": {
      "score": 32,
      "unit": "centipawn",
      "perspective": "w"
    },
    "search": {
      "depth": 4,
      "nodes": 18234,
      "timeMs": 86,
      "pv": [{ "uci": "h2e2" }]
    }
  }
}
```

`move` 是唯一需要消费的字段。调用方自行落子后，把新局面再次提交给接口即可。

### `POST /api/v1/apply-move`

无状态地把一步棋应用到当前局面，返回新棋盘、轮次、状态和全部合法走法。`/play` 页面用它更新人类和 AI 的落子。

```json
{
  "game": "xiangqi",
  "position": { "fen": "rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1" },
  "move": { "uci": "h2e2" }
}
```

`move` 可以只传 `uci`，也可以传 `index`，或 `from` / `to`（扁平索引）。

### `POST /api/v1/legal-moves`

返回指定局面的全部合法走法，便于前端做落点提示或校验。请求体与 `move` 相同，不需要难度和时间。

```json
{
  "game": "xiangqi",
  "fen": "rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1"
}
```

### `POST /api/v1/move/stream`（SSE）

同样的局面请求，改成 `text/event-stream`，把迭代加深的每一层实时推给调用方。高难度（`hard` / `master`）时尤其有用。

```bash
curl -N -X POST http://127.0.0.1:8090/api/v1/move/stream \
  -H "Content-Type: application/json" \
  -d '{
    "game": "xiangqi",
    "fen": "rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1",
    "difficulty": "master",
    "timeLimitMs": 6000
  }'
```

事件：

| event | data | 说明 |
|---|---|---|
| `start` | game / engine / difficulty / timeLimitMs / maxDepth / turn | 思考开始（`engine` 为本次实际选择的引擎） |
| `depth` | depth / score / nodes / timeMs / best / pv | 每完成一层迭代加深 |
| `result` | 与 `/api/v1/move` 的 `data` 相同 | 最终走法，随后连接关闭 |
| `error` | ok:false / error.code / error.message | 搜索或参数错误 |
| `done` | ok / timeMs / depth / nodes / gameOver | 流结束标记 |

服务端每 5 秒发送一条 SSE 注释心跳。客户端断开连接后，搜索会收到取消信号并停止。

FEN 棋种也可以直接用 `EventSource`：

```js
const fen = "rnbakabnr/9/1c5c1/...";
const url = `/api/v1/move/stream?game=xiangqi&fen=${encodeURIComponent(fen)}&difficulty=master&timeLimitMs=6000`;
const es = new EventSource(url);
es.addEventListener("depth", (e) => {
  const p = JSON.parse(e.data);
  console.log(`depth ${p.depth}, score ${p.score}`, p.best);
});
es.addEventListener("result", (e) => {
  console.log("best move", JSON.parse(e.data).move);
  es.close();
});
```

网格棋种的 `board` 数组较大，建议使用 `POST` + `fetch` 读取流；也可以把完整 JSON 放进 URL 编码的 `payload` 查询参数：

```
GET /api/v1/move/stream?payload=%7B%22game%22%3A%22gomoku%22%2C...%7D
```

### 其他接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/health` | 健康检查、版本、已加载棋种数 |
| GET | `/api/v1/games` | 棋种目录、棋盘元数据、初始局面、规则 |
| GET | `/api/v1/games/{game}` | 单个棋种元数据 |
| POST | `/api/v1/move` | 计算最佳走法 |
| POST | `/api/v1/games/{game}/move` | 同上，棋种在路径中 |
| POST | `/api/v1/apply-move` | 应用一步棋，返回新局面 |
| POST | `/api/v1/games/{game}/apply-move` | 同上，棋种在路径中 |
| GET/POST | `/api/v1/move/stream` | SSE 流式思考过程，最后返回 `result` |
| GET/POST | `/api/v1/games/{game}/move/stream` | 同上，棋种在路径中 |
| POST | `/api/v1/legal-moves` | 列出合法走法 |
| POST | `/api/v1/games/{game}/legal-moves` | 同上，棋种在路径中 |

请求体也可以不用嵌套的 `position`，直接把 `fen` / `board` / `turn` 放在顶层：

```json
{
  "game": "gomoku",
  "board": ["", "", "... 共 225 格"],
  "turn": "b",
  "difficulty": "hard"
}
```

---

## 请求字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `game` | string | 是* | 棋种 id；使用路径形式时可省略 |
| `position.fen` | string | 视棋种 | 国际象棋 / 中国象棋 |
| `position.board` | string[] | 视棋种 | 网格棋种，行优先扁平数组，空格用 `""` |
| `position.turn` | string | 视棋种 | 行棋方 |
| `difficulty` | string | 否 | `easy` / `normal` / `hard` / `master`，默认 `normal` |
| `engine` | string | 否 | 选择搜索引擎，见下方 [引擎选择](#引擎选择)；默认使用棋种默认引擎 |
| `timeLimitMs` | int | 否 | 50–15000，默认随难度 |
| `maxDepth` | int | 否 | 1–10，不传则按难度 |
| `seed` | int64 | 否 | 随机种子；用于部分棋种在等价走法/立即获胜走法中的选择 |

`fen` / `board` / `turn` 也可以放在顶层。嵌套的 `position` 优先。

### 引擎选择

`engine` 字段允许按请求挑选搜索引擎（SSE 的 `start` 事件会回显所选引擎）：

| 棋种 | 可选引擎（默认在前） |
|---|---|
| 中国象棋 | 加载 NNUE 时 `pikafish-nnue`、`godogpaw`、`builtin`；否则 `godogpaw`、`builtin` |
| 国际象棋 | `chess-3`、`builtin` |
| 其余棋种 | `builtin` |

- 传空或 `"auto"` 使用默认引擎；
- `godogpaw` / `chess-3` / `builtin` 始终可用；`pikafish-nnue` 需要先配置 NNUE 网络；
- 引擎名非法或不可用时返回 `400`、`UNKNOWN_ENGINE`，错误消息列出可选值；
- `/api/v1/games` 返回每个棋种的 `engines` 列表，可用它做动态下拉。

```json
{ "game": "xiangqi", "fen": "...", "engine": "godogpaw", "difficulty": "master" }
```

### 响应字段

| 字段 | 说明 |
|---|---|
| `engine` | 实际使用的引擎：`chess-3`、`godogpaw`、`pikafish-nnue` 或内置 `builtin` |
| `move` | 最佳走法。`uci` 用于 FEN 棋种；`index` 用于落子类棋种；`from` / `to` 用于移动类棋种 |
| `status` | 走完这一步后的状态：`playing`、`check`、`checkmate`、`stalemate`、`win`、`draw` |
| `gameOver` | 该走法是否结束对局 |
| `evaluation` | 搜索评分（centipawn）与视角；将杀时含 `mateIn` |
| `search` | 实际深度、节点数、耗时、是否超时、PV 前缀 |

当局面已经终局时，接口返回 `200`、`move: null`、`gameOver: true`，而不是报错。

---

## 支持的棋种

| id | 棋种 | 棋盘 | 局面格式 | 走法格式 | 先手 |
|---|---|---|---|---|---|
| `chess` | 国际象棋 | 8×8 | FEN | UCI（`e2e4`，升变 `e7e8q`） | 白 `w` |
| `xiangqi` | 中国象棋 | 9×10 | FEN | UCCI（`h2e2`） | 红 `w` |
| `gomoku` | 五子棋 | 15×15 | `board[225]` | `{"index": 落点}` | 黑 `b` |
| `reversi` | 黑白棋 | 8×8 | `board[64]` | `{"index": 落点}` | 黑 `b` |
| `connect4` | 四子棋 | 7×6 | `board[42]` | `{"index": 落点}`（该列最低空位） | 红 `r` |
| `jungle` | 斗兽棋 | 7×9 | `board[63]`，如 `"rE"` | `{"from": 起点, "to": 终点}` | 红 `r` |
| `checkers` | 中国跳棋 | 121 孔 | `board[121]` | `{"from": 起点, "to": 终点}` | 红 `r` |

别名：`chinese-chess` → `xiangqi`、`international-chess` → `chess`、`othello` → `reversi`、`connect-four` → `connect4`、`chinese-checkers` → `checkers` 等。

### FEN 棋种

- **国际象棋**：标准 FEN；行棋方 `w` / `b`。走法为 UCI，升变加 `q/r/b/n`。
- **中国象棋**：标准象棋 FEN；行棋方 `w`（红）/ `b`（黑）。走法为 UCCI，坐标从红方底线起算（`a0`–`i9`）。

### 网格棋种

`board` 是行优先扁平数组，`index = row * cols + col`，`row 0` 在棋盘上方。空格写 `""`（JSON 的 `null` 也按空处理）。

- `gomoku`：`"b"` / `"w"` / `""`，225 格。
- `reversi`：`"b"` / `"w"` / `""`，64 格。
- `connect4`：`"r"` / `"y"` / `""`，42 格。`index` 必须是该列最下方的空位；例如空棋盘中间列首手是 `index = 5*7+3 = 38`。
- `jungle`：`"rE"` / `"bR"` 这种“颜色 + 兽种”格式，63 格。兽种：`E` 象、`L` 狮、`T` 虎、`P` 豹、`W` 狼、`D` 狗、`C` 猫、`R` 鼠。
- `checkers`：`"r"` / `"b"` / `""`，121 孔。`from` / `to` 是孔位编号；坐标为 `row`（0–16）与 `col = x2`（可为负），只是辅助展示。

初始局面可以从 `GET /api/v1/games` 的 `initialPosition` 直接复制。

---

## 难度与时间

| difficulty | 默认深度 | 默认时间 |
|---|---|---|
| `easy` | 2 | 300 ms |
| `normal` | 4 | 1000 ms |
| `hard` | 5 | 2500 ms |
| `master` | 7 | 6000 ms |

各棋种会按分支数调整实际上限。`timeLimitMs` 和 `maxDepth` 可以覆盖默认值。搜索是时间预算，不保证精确耗时；迭代加深会在超时前保留上一层的完整结果。

---

## 错误响应

```json
{
  "ok": false,
  "error": {
    "code": "INVALID_POSITION",
    "message": "象棋 FEN 棋盘必须有 10 行，实际 8 行"
  }
}
```

| HTTP | code | 含义 |
|---|---|---|
| 400 | `BAD_REQUEST` | 缺少参数、JSON 不合法 |
| 400 | `INVALID_POSITION` | 棋盘 / FEN 格式错误或局面不合法 |
| 400 | `UNKNOWN_ENGINE` | 引擎名不存在或当前不可用（如未加载 NNUE 时请求 `pikafish-nnue`） |
| 404 | `UNKNOWN_GAME` | 棋种 id 不存在 |
| 504 | `TIMEOUT` | 搜索超时且没有可用走法 |
| 500 | `INTERNAL_ERROR` | 服务内部错误 |

当局面已经终局时，`move` 接口返回 `200`、`move: null`、`gameOver: true`，而不是 HTTP 错误。

---

## 配置

`application.yml`：

```yaml
web:
  server:
    port: 8090
    contextPath: auto-chess
    max_body_size: 1048576
    read_timeout: 30
  log:
    level: info
engine:
  xiangqi:
    nnue: pikafish.nnue
```

环境变量：

- `PORT`：覆盖端口
- `CONTEXT_PATH`：覆盖 contextPath，例如 `/games` 或 `games`
- `XIANGQI_NNUE`：覆盖中国象棋 NNUE 文件路径

### Pikafish NNUE（可选）

中国象棋可以加载 Pikafish 2024-03-10 的官方 NNUE 网络。

下载地址：

- 发布页：<https://github.com/official-pikafish/Pikafish/releases/tag/Pikafish-2024-03-10>
- 直接下载：<https://github.com/official-pikafish/Pikafish/releases/download/Pikafish-2024-03-10/Pikafish.2024-03-10.7z>

安装步骤：

1. 下载 `Pikafish.2024-03-10.7z` 并解压发布包；
2. 发布包中的 `pikafish.nnue` 本身是一个 ZIP 包，需要再解压一次，得到真正的 NNUE 文件；
3. 把解压后的 `pikafish.nnue` 放到项目根目录，或修改 `engine.xiangqi.nnue` / `XIANGQI_NNUE` 指向它；
4. 正确的 NNUE 文件大小应为 **43,110,204 字节**，SHA-256 为：

```text
A5AAB76B978DA545A4F2553F9C6DCE2A36AE29C7A17479139266E8AF5F5B109E
```

Windows 校验：

```powershell
Get-FileHash .\pikafish.nnue -Algorithm SHA256
```

加载成功后，`/api/v1/move` 默认返回的 `engine` 为 `pikafish-nnue`，此时仍可通过 `"engine": "godogpaw"` 或 `"engine": "builtin"` 按请求切换（见 [引擎选择](#引擎选择)）；文件不存在或解析失败时自动回退 `godogpaw`，不会阻塞启动。

权重文件不提交到仓库。该实现是纯 Go 的 NNUE 推理，不调用 exe、CGo 或浏览器 WASM。

页面链接、静态资源和前端 API 请求都按当前页面深度生成相对路径，因此修改 `contextPath` 后无需修改模板或 JS。

跨域：全局 `CORSFilter` 对所有已注册接口统一加头（包括普通 JSON、SSE、错误响应和静态文档资源），`OPTIONS` 预检返回 `204`。默认允许 `*`，并允许 `Last-Event-ID`，便于 EventSource 重连。

---

## 测试

```bash
go test ./...
```

覆盖内容：

- 国际象棋 perft（初始局面深度 3、Kiwipete 深度 2）、王车易位、吃过路兵、升变、将杀/逼和
- 中国象棋 perft（44 / 1920）、炮架、蹩马腿、将帅照面
- 五子棋五连判定、立即取胜、封堵活四
- 黑白棋初始走法、翻转、终局计子
- 四子棋重力落下、纵向连四、立即取胜
- 斗兽棋鼠象克制、水中鼠、狮虎跳河、陷阱、兽穴
- 中国跳棋几何、连跳、十子归营
- HTTP 接口、别名、错误码、CORS、多页面模板渲染、`/play` 对弈页
- 对弈页：七种棋盘渲染、人机轮流走子、SSE 思考过程、本机保存/继续/悔棋
- 亮色/深色主题切换与中文/English 全站切换
- 默认亮色、主题记忆、语言 cookie / `?lang=en` 与英文页面渲染测试
- 每个棋种的接口文档：字段表、请求/响应、apply-move、legal-moves、SSE、注意事项
- SSE 事件序列（start / depth / result / done）、GET EventSource 形式、各棋种 progress 回调
- 原生引擎搜索：Chess-3（国际象棋）与 godogpaw（中国象棋）返回合法走法、引擎名和搜索统计

---

## 目录

```
.
├── main.go                  # go-web-frame 启动入口
├── application.yml          # 端口、日志、请求体限制
├── internal/
│   ├── api/                 # 路由、CORS、JSON 响应、文档页
│   ├── game/                # 通用接口与 JSON 类型
│   ├── games/
│   │   ├── chess/           # 国际象棋：FEN + 完整规则 + Chess-3 纯 Go 引擎
│   │   ├── xiangqi/         # 中国象棋：FEN/UCCI + 完整规则 + godogpaw 纯 Go 引擎
│   │   ├── gomoku/          # 五子棋：威胁检测 + 候选裁剪
│   │   ├── reversi/         # 黑白棋：位棋盘 + Negamax
│   │   ├── connect4/        # 四子棋：Alpha-Beta
│   │   ├── jungle/          # 斗兽棋：Alpha-Beta
│   │   └── checkers/        # 中国跳棋：最短路径评估 + 束搜索
│   ├── registry/            # 棋种注册与别名
│   ├── engines/
│   │   └── godogpaw/        # 纯 Go 中国象棋引擎（MIT，含 LICENSE）
│   └── webui/               # Go html/template 多页面站点
│       ├── templates/       # layout、首页、指南、接口、SSE、棋种、调试页
│       ├── static/          # app.css、site.js、playground.js
│       └── webui.go         # 模板渲染器 / 嵌入资源
└── ...
```

## 实现说明

- HTTP 层使用 go-web-frame 的 `Builder`、`Rest`、`Filter`、`Config`、`Run`。
- 页面使用 Go `html/template`：`layout` 统一导航/页脚，各页面模板只写内容；棋种目录和详情由 `registry` 数据直接渲染。
- 所有引擎实现 `internal/game.Game`：`Decode` / `Encode` / `LegalMoves` / `Apply` / `Search`。
- 国际象棋接入 [Chess-3](https://github.com/paulsonkoly/chess-3)（纯 Go），中国象棋接入 [godogpaw](https://github.com/hmgle/godogpaw)（纯 Go）；可选加载 Pikafish 2024 NNUE 网络。
- NNUE 推理为纯 Go 实现：读取 `.nnue`、解压 LEB128、Feature Transformer、8 个 LayerStack，不依赖 CGo、WASM 或原生引擎进程。
- 接口层只做参数归一化、错误映射和 JSON 序列化，不保存任何对局状态。
- 搜索均为纯 Go；交叉编译到 Windows / Linux / macOS 时不需要额外安装引擎。

## 限制

- 不保存棋谱，因此无法检测三次重复局面；需要的话请由调用方根据历史判断终局。
- 棋钟、50 回合、60 回合等规则由 FEN 中的半回合计数提供，服务端只读取当前值。
- 国际象棋使用 Chess-3（纯 Go）；中国象棋默认 godogpaw，也可加载 Pikafish 2024 NNUE 网络；NNUE 评估已支持增量累加器、置换表、Zobrist、空着裁剪和 LMR，但搜索仍是 Go 实现，棋力低于 Pikafish 原版。
- Pikafish NNUE 网络文件不随仓库分发，需要自行下载；缺少文件时自动回退 godogpaw。
- 公开接口没有鉴权与限流，部署到公网时请在前面加网关或反向代理。

## 第三方引擎许可

- [Chess-3](https://github.com/paulsonkoly/chess-3)：MIT License。
- [godogpaw](https://github.com/hmgle/godogpaw)：MIT License，源码位于 `internal/engines/godogpaw/`，许可证随源码保留。
- [Pikafish](https://github.com/official-pikafish/Pikafish)：GPL-3.0；NNUE 权重遵循 Pikafish 项目的 NNUE License。仓库不包含官方权重文件。
