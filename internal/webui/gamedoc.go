package webui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/chuccp/auto-chess/internal/game"
)

// DocField is one row in a game-specific API field table.
type DocField struct {
	Name string
	Type string
	Desc string
}

// LegendItem is one piece/empty-cell code used by a game.
type LegendItem struct {
	Code    string
	Meaning string
}

// GameDoc is the detailed, game-specific API documentation shown on
// /games/{game}. It is generated from the same metadata and rules the server
// uses, so examples and field descriptions cannot drift from the engine.
type GameDoc struct {
	RequestJSON       string
	Curl              string
	MoveResponseJSON  string
	ApplyRequestJSON  string
	ApplyResponseJSON string
	ApplyCurl         string
	LegalRequestJSON  string
	LegalResponseJSON string
	SSEURL            string
	SSECurl           string
	SSEExample        string
	MoveInputJSON     string
	MoveOutputJSON    string
	Fields            []DocField
	Notes             []string
	Legend            []LegendItem
	BoardSummary      string
	TurnSummary       string
	IndexSummary      string
	Engine            string
}

// BuildGameDoc builds detailed examples for one game. g must be the concrete
// engine for meta, because the apply response is produced by actually applying
// the first legal move.
func BuildGameDoc(meta *game.Meta, g game.Game, lang ...string) *GameDoc {
	language := "zh"
	if len(lang) > 0 {
		language = NormalizeLang(lang[0])
	}
	doc := &GameDoc{
		Engine:       engineLabel(meta.ID),
		BoardSummary: boardSummary(meta, language),
		TurnSummary:  turnSummary(meta, language),
		IndexSummary: meta.Board.Origin,
		Fields:       gameFields(meta),
		Notes:        gameNotes(meta),
		Legend:       legend(meta),
		SSEURL:       fmt.Sprintf("/api/v1/games/%s/move/stream", meta.ID),
	}
	if len(meta.InitialPosition.LegalMoves) == 0 {
		return doc
	}
	first := meta.InitialPosition.LegalMoves[0]
	moveInput := moveToInput(first)

	posInput := game.PositionInput{
		FEN:   meta.InitialPosition.FEN,
		Board: meta.InitialPosition.Board,
		Turn:  meta.InitialPosition.Turn,
	}
	request := map[string]any{
		"game":        meta.ID,
		"position":    positionForRequest(posInput),
		"difficulty":  "normal",
		"timeLimitMs": 1000,
	}
	doc.RequestJSON = prettyJSON(request)
	doc.Curl = curlMove(meta.ID, request)
	doc.MoveInputJSON = prettyJSON(moveInput)
	doc.MoveOutputJSON = prettyJSON(first)

	response := map[string]any{
		"ok": true,
		"data": map[string]any{
			"game":       meta.ID,
			"difficulty": "normal",
			"engine":     doc.Engine,
			"move":       first,
			"gameOver":   false,
			"status":     game.Status{State: "playing"},
			"evaluation": map[string]any{
				"score": 18, "unit": "centipawn", "perspective": meta.InitialPosition.Turn,
			},
			"search": map[string]any{"depth": 8, "nodes": 42000, "timeMs": 86},
		},
	}
	doc.MoveResponseJSON = prettyJSON(response)

	applyReq := map[string]any{
		"game":     meta.ID,
		"position": positionForRequest(posInput),
		"move":     moveInput,
	}
	doc.ApplyRequestJSON = prettyJSON(applyReq)
	doc.ApplyCurl = curlApply(meta.ID, applyReq)

	var pos game.Position
	var err error
	if g != nil {
		pos, err = g.Decode(posInput)
	}
	if err == nil && g != nil {
		next, applied, applyErr := g.Apply(pos, moveInput)
		if applyErr == nil {
			view := g.Encode(next, true)
			view = trimView(view, 2)
			doc.ApplyResponseJSON = prettyJSON(map[string]any{
				"ok": true,
				"data": map[string]any{
					"game":     meta.ID,
					"move":     applied,
					"position": view,
				},
			})
		}
	}
	if doc.ApplyResponseJSON == "" {
		doc.ApplyResponseJSON = prettyJSON(map[string]any{
			"ok": true,
			"data": map[string]any{
				"game": meta.ID,
				"move": first,
				"position": map[string]any{
					"fen": meta.InitialPosition.FEN, "turn": meta.InitialPosition.Turn,
					"status": game.Status{State: "playing"}, "legalMoves": []game.Move{first},
				},
			},
		})
	}

	legalReq := map[string]any{"game": meta.ID, "position": positionForRequest(posInput)}
	doc.LegalRequestJSON = prettyJSON(legalReq)
	doc.LegalResponseJSON = prettyJSON(map[string]any{
		"ok": true,
		"data": map[string]any{
			"game":     meta.ID,
			"count":    meta.InitialPosition.LegalMoveCount,
			"position": trimView(meta.InitialPosition, 3),
		},
	})

	doc.SSECurl = curlStream(meta.ID, request)
	doc.SSEExample = sseExample(meta, first, doc.Engine)
	return doc
}

func engineLabel(id string) string {
	switch id {
	case "chess":
		return "chess-3"
	case "xiangqi":
		return "godogpaw"
	}
	return "builtin"
}

func boardSummary(meta *game.Meta, lang string) string {
	origin := meta.Board.Origin
	if lang == "en" {
		if translated, ok := enText[strings.TrimSpace(origin)]; ok {
			origin = translated
		}
		if meta.Board.Type == "fen" {
			return fmt.Sprintf("FEN; %d×%d board; %s", meta.Board.Cols, meta.Board.Rows, origin)
		}
		return fmt.Sprintf("board[%d]; %d rows × %d cols; %s", meta.Board.Cells, meta.Board.Rows, meta.Board.Cols, origin)
	}
	if meta.Board.Type == "fen" {
		return fmt.Sprintf("FEN；%d×%d 棋盘；%s", meta.Board.Cols, meta.Board.Rows, origin)
	}
	return fmt.Sprintf("board[%d]；%d 行 × %d 列；%s", meta.Board.Cells, meta.Board.Rows, meta.Board.Cols, origin)
}

func turnSummary(meta *game.Meta, lang string) string {
	if lang == "en" {
		parts := make([]string, 0, len(meta.Colors))
		for _, color := range meta.Colors {
			parts = append(parts, fmt.Sprintf("%s=%s", color.ID, enColorName(meta.ID, color.ID)))
		}
		return strings.Join(parts, ", ") + "; first move " + meta.InitialPosition.Turn
	}
	parts := make([]string, 0, len(meta.Colors))
	for _, color := range meta.Colors {
		parts = append(parts, fmt.Sprintf("%s=%s", color.ID, color.Name))
	}
	return strings.Join(parts, "，") + "；首手 " + meta.InitialPosition.Turn
}

func enColorName(gameID, colorID string) string {
	if gameID == "xiangqi" {
		if colorID == "w" {
			return "Red"
		}
		return "Black"
	}
	switch colorID {
	case "w":
		return "White"
	case "b":
		return "Black"
	case "r":
		return "Red"
	case "y":
		return "Yellow"
	}
	return colorID
}

func legend(meta *game.Meta) []LegendItem {
	if len(meta.Board.Legend) == 0 {
		return nil
	}
	codes := make([]string, 0, len(meta.Board.Legend))
	for code := range meta.Board.Legend {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	out := make([]LegendItem, 0, len(codes))
	for _, code := range codes {
		label := code
		if label == "" {
			label = "（空）"
		}
		out = append(out, LegendItem{Code: label, Meaning: meta.Board.Legend[code]})
	}
	return out
}

func positionForRequest(in game.PositionInput) map[string]any {
	if in.FEN != "" {
		return map[string]any{"fen": in.FEN}
	}
	return map[string]any{"board": in.Board, "turn": in.Turn}
}

func moveToInput(m game.Move) game.MoveInput {
	switch {
	case m.UCI != "":
		return game.MoveInput{UCI: m.UCI}
	case m.Index != nil:
		idx := m.Index.Index
		return game.MoveInput{Index: &idx}
	case m.From != nil && m.To != nil:
		from, to := m.From.Index, m.To.Index
		return game.MoveInput{From: &from, To: &to, Promotion: m.Promotion}
	}
	return game.MoveInput{}
}

func trimView(v game.PositionView, n int) game.PositionView {
	if len(v.LegalMoves) > n {
		v.LegalMoves = append([]game.Move(nil), v.LegalMoves[:n]...)
	}
	return v
}

func compact(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func curlMove(id string, request map[string]any) string {
	return fmt.Sprintf("curl -X POST http://127.0.0.1:8090/api/v1/games/%s/move \\\n  -H \"Content-Type: application/json\" \\\n  -d '%s'", id, compact(request))
}

func curlApply(id string, request map[string]any) string {
	return fmt.Sprintf("curl -X POST http://127.0.0.1:8090/api/v1/games/%s/apply-move \\\n  -H \"Content-Type: application/json\" \\\n  -d '%s'", id, compact(request))
}

func curlStream(id string, request map[string]any) string {
	return fmt.Sprintf("curl -N -X POST http://127.0.0.1:8090/api/v1/games/%s/move/stream \\\n  -H \"Content-Type: application/json\" \\\n  -d '%s'", id, compact(request))
}

func sseExample(meta *game.Meta, move game.Move, engineName string) string {
	return fmt.Sprintf(`event: start
data: {"game":"%s","engine":"%s","difficulty":"normal","maxDepth":0,"turn":"%s"}

event: depth
data: {"depth":4,"score":18,"nodes":12000,"timeMs":35,"best":%s}

event: result
data: {"game":"%s","engine":"%s","move":%s,"gameOver":false,"status":{"state":"playing"}}

event: done`, meta.ID, engineName, meta.InitialPosition.Turn, compact(move), meta.ID, engineName, compact(move))
}

func gameFields(meta *game.Meta) []DocField {
	fields := []DocField{
		{Name: "game", Type: "string", Desc: "棋种 id；使用路径形式 /api/v1/games/" + meta.ID + "/move 时可省略。"},
	}
	if meta.Board.Type == "fen" {
		fields = append(fields,
			DocField{Name: "position.fen", Type: "string", Desc: "标准 FEN；轮次、易位权、吃过路兵/半回合计数都写在 FEN 中。"},
			DocField{Name: "position.turn", Type: "string", Desc: "通常不用单独传；只在没有 FEN 轮次字段时作为补充。"},
		)
	} else {
		fields = append(fields,
			DocField{Name: "position.board", Type: "string[]", Desc: fmt.Sprintf("长度 %d 的行优先数组；空格用空字符串。", meta.Board.Cells)},
			DocField{Name: "position.turn", Type: "string", Desc: "当前行棋方：" + colorIDs(meta) + "。"},
		)
	}
	switch meta.ID {
	case "chess":
		fields = append(fields,
			DocField{Name: "move.uci", Type: "string", Desc: "UCI 走法，如 e2e4；升变例 e7e8q。"},
			DocField{Name: "move.promotion", Type: "string", Desc: "升变子 q / r / b / n；也可只写在 uci 末尾。"},
		)
	case "xiangqi":
		fields = append(fields, DocField{Name: "move.uci", Type: "string", Desc: "UCCI 走法，如 h2e2、b0c2；从红方底线 a0 起算。"})
	case "gomoku":
		fields = append(fields, DocField{Name: "move.index", Type: "int", Desc: "落子交叉点下标 0–224；index = row*15 + col。"})
	case "reversi":
		fields = append(fields, DocField{Name: "move.index", Type: "int", Desc: "落子格子下标 0–63；必须能翻转至少一枚对方棋子。"})
	case "connect4":
		fields = append(fields, DocField{Name: "move.index", Type: "int", Desc: "落点格子下标 0–41；必须是该列最下方的空位。"})
	case "jungle":
		fields = append(fields,
			DocField{Name: "move.from", Type: "int", Desc: "起点扁平下标 0–62。"},
			DocField{Name: "move.to", Type: "int", Desc: "终点扁平下标 0–62。"},
		)
	case "checkers":
		fields = append(fields,
			DocField{Name: "move.from", Type: "int", Desc: "起点孔位编号 0–120。"},
			DocField{Name: "move.to", Type: "int", Desc: "终点孔位编号 0–120；单步或一次跳跃。"},
		)
	}
	fields = append(fields,
		DocField{Name: "difficulty", Type: "string", Desc: "easy / normal / hard / master，默认 normal。"},
		DocField{Name: "timeLimitMs", Type: "int", Desc: "思考时间上限 50–15000ms；不传按难度。"},
		DocField{Name: "search.depth", Type: "int", Desc: "响应中的实际思考深度。"},
		DocField{Name: "search.nodes", Type: "int64", Desc: "响应中的思考节点数。"},
		DocField{Name: "evaluation", Type: "object", Desc: "评分（centipawn），视角为走子一方；将杀含 mateIn。"},
	)
	return fields
}

func colorIDs(meta *game.Meta) string {
	parts := make([]string, 0, len(meta.Colors))
	for _, color := range meta.Colors {
		parts = append(parts, color.ID)
	}
	return strings.Join(parts, " / ")
}

func gameNotes(meta *game.Meta) []string {
	notes := []string{
		"接口无状态：每次请求都携带完整局面，服务端不保存上一手或对局历史。",
		"响应中的 move 是走子对象，不是新棋盘；需要新棋盘时调用 /api/v1/apply-move。",
	}
	switch meta.ID {
	case "chess":
		notes = append(notes,
			"FEN 中的 side-to-move 必须与当前行棋方一致；易位权、吃过路兵格也必须保留在 FEN 中。",
			"王车易位、吃过路兵、兵升变由规则自动处理；升变走法在 UCI 末尾带 q/r/b/n。",
			"不根据历史检测三次重复；50 回合计数从 FEN 读取。",
		)
	case "xiangqi":
		notes = append(notes,
			"UCCI 坐标从红方底线起算：a0–i9；FEN 则按标准从上到下书写，红方在下方。",
			"将帅照面、蹩马腿、塞象眼、炮架都已包含在合法走法判断中。",
			"不根据历史检测长将/重复局面；60 回合计数从 FEN 读取。",
		)
	case "gomoku":
		notes = append(notes,
			"board 长度必须是 225；index = row*15 + col，row 0 为最上行。",
			"自由五子规则：连续五子或更多即获胜，没有禁手限制。",
			"落子位置必须是空格；已有棋子的 index 会返回 BAD_MOVE/INVALID_MOVE。",
		)
	case "reversi":
		notes = append(notes,
			"board 长度必须是 64；index = row*8 + col，row 0 为最上行。",
			"只有能翻转至少一枚对方棋子的落点才是合法走法。",
			"一方无子可下时引擎自动跳过；响应 status 会反映轮次变化，客户端继续用返回的 turn 请求即可。",
		)
	case "connect4":
		notes = append(notes,
			"board 长度必须是 42；index = row*7 + col，row 0 为最上行。",
			"落子必须落在该列最下方的空位；建议直接用合法走法返回的 index。",
			"胜负判定覆盖横、竖和两条斜线。",
		)
	case "jungle":
		notes = append(notes,
			"board 长度必须是 63；row 0 为黑方底线（上方），index = row*7 + col。",
			"棋子编码为颜色+兽名，如 rE（红象）、bR（黑鼠）；空位为 \"\"。",
			"河流、陷阱、兽穴、鼠吃象、狮虎跳河都由规则处理；移动用 from/to 两个扁平下标。",
		)
	case "checkers":
		notes = append(notes,
			"board 长度必须是 121；孔位按 17 行排列，行孔数为 1,2,3,4,13,12,11,10,9,10,11,12,13,4,3,2,1。",
			"移动用 from/to 两个孔位编号；跳跃可以跨越任意颜色棋子，被跨越的棋子不动。",
			"连跳按引擎返回的当前一步应用；如果还有后续跳跃，下一次合法走法会继续给出。",
			"先把 10 枚棋子全部走进对面角阵营者获胜；无合法走法判负。",
		)
	}
	return notes
}
