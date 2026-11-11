// Package ops reads the whole state out of SQLite and applies validated,
// transactional batches of changes.
package ops

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"
)

const (
	MaxText  = 10_000
	MaxImage = 2_200_000
	MaxOrd   = 1_000_000
	MaxOps   = 5_000
)

var (
	idPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)
	datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	intPattern  = regexp.MustCompile(`^-?[0-9]+$`)
)

// Querier is satisfied by both *sql.DB and *sql.Tx.
type Querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

type InvalidOperation struct {
	Message string
}

func (e *InvalidOperation) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &InvalidOperation{Message: fmt.Sprintf(format, args...)}
}

// ------------------------------------------------------------------ state read

type Question struct {
	ID         string `json:"id"`
	Subject    string `json:"subject"`
	Title      string `json:"title"`
	Answer     string `json:"answer"`
	Reason     string `json:"reason"`
	Tag        string `json:"tag"`
	Cause      string `json:"cause"`
	Difficulty string `json:"difficulty"`
	Image      string `json:"image"`
	Favorite   bool   `json:"favorite"`
	Stage      int    `json:"stage"`
	Due        string `json:"due"`
	Created    string `json:"created"`
	Demo       bool   `json:"demo"`
}

type Log struct {
	ID      string   `json:"id"`
	Date    string   `json:"date"`
	At      string   `json:"at"`
	Type    string   `json:"type"`
	Label   string   `json:"label"`
	Points  float64  `json:"points"`
	QID     *string  `json:"qid,omitempty"`
	Grade   *string  `json:"grade,omitempty"`
	Minutes *float64 `json:"minutes,omitempty"`
}

type Reward struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Cost int    `json:"cost"`
}

type Redemption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Cost int    `json:"cost"`
	Date string `json:"date"`
}

type Prefs struct {
	Name    string `json:"name"`
	Goal    int    `json:"goal"`
	Minutes int    `json:"minutes"`
	Theme   string `json:"theme"`
}

type Timer struct {
	Running   bool     `json:"running"`
	Remaining float64  `json:"remaining"`
	Duration  float64  `json:"duration"`
	End       *float64 `json:"end,omitempty"`
}

type AISetQuestion struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Answer      string `json:"answer"`
	Explanation string `json:"explanation"`
	Knowledge   string `json:"knowledge"`
	Note        string `json:"note"`
	Revealed    bool   `json:"revealed"`
	Grade       string `json:"grade"`
}

type AISet struct {
	ID          string          `json:"id"`
	AIReviewed  bool            `json:"aiReviewed"`
	SourceID    string          `json:"sourceId"`
	SourceTitle string          `json:"sourceTitle"`
	Subject     string          `json:"subject"`
	Created     string          `json:"created"`
	Questions   []AISetQuestion `json:"questions"`
}

type AIConfig struct {
	Provider       *string `json:"provider"`
	Base           *string `json:"base"`
	Vision         *string `json:"vision"`
	Text           *string `json:"text"`
	DeepSeekBase   *string `json:"deepseekBase"`
	DeepSeekVision *string `json:"deepseekVision"`
	DeepSeekText   *string `json:"deepseekText"`
}

type State struct {
	Schema      int          `json:"schema"`
	Questions   []Question   `json:"questions"`
	Trash       []Question   `json:"trash"`
	Logs        []Log        `json:"logs"`
	Rewards     []Reward     `json:"rewards"`
	Redemptions []Redemption `json:"redemptions"`
	Prefs       Prefs        `json:"prefs"`
	Timer       *Timer       `json:"timer"`
	AISets      []AISet      `json:"aiSets"`
	AIConfig    *AIConfig    `json:"aiConfig,omitempty"`
	Initialized bool         `json:"initialized"`
}

// ------------------------------------------------------------------- read state

func ReadState(q Querier, owner string) (*State, error) {
	state := &State{
		Schema:      1,
		Questions:   []Question{},
		Trash:       []Question{},
		Logs:        []Log{},
		Rewards:     []Reward{},
		Redemptions: []Redemption{},
		Prefs:       Prefs{Name: "同学", Goal: 5, Minutes: 25, Theme: "light"},
		AISets:      []AISet{},
	}

	rows, err := q.Query(`SELECT id, trashed, subject, title, answer, reason, tag, cause,
		difficulty, image, favorite, stage, due, created, demo
		FROM questions WHERE owner_id = ? ORDER BY ord`, owner)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item Question
		var trashed bool
		if err := rows.Scan(&item.ID, &trashed, &item.Subject, &item.Title, &item.Answer,
			&item.Reason, &item.Tag, &item.Cause, &item.Difficulty, &item.Image,
			&item.Favorite, &item.Stage, &item.Due, &item.Created, &item.Demo); err != nil {
			rows.Close()
			return nil, err
		}
		if trashed {
			state.Trash = append(state.Trash, item)
		} else {
			state.Questions = append(state.Questions, item)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	rows, err = q.Query(`SELECT id, date, at, type, label, points, qid, grade, minutes
		FROM logs WHERE owner_id = ? ORDER BY ord`, owner)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item Log
		var qid, grade sql.NullString
		var minutes sql.NullFloat64
		if err := rows.Scan(&item.ID, &item.Date, &item.At, &item.Type, &item.Label,
			&item.Points, &qid, &grade, &minutes); err != nil {
			rows.Close()
			return nil, err
		}
		if qid.Valid {
			value := qid.String
			item.QID = &value
		}
		if grade.Valid {
			value := grade.String
			item.Grade = &value
		}
		if minutes.Valid {
			value := minutes.Float64
			item.Minutes = &value
		}
		state.Logs = append(state.Logs, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	rows, err = q.Query(`SELECT id, name, cost FROM rewards WHERE owner_id = ? ORDER BY ord`, owner)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item Reward
		if err := rows.Scan(&item.ID, &item.Name, &item.Cost); err != nil {
			rows.Close()
			return nil, err
		}
		state.Rewards = append(state.Rewards, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	rows, err = q.Query(`SELECT id, name, cost, date FROM redemptions WHERE owner_id = ? ORDER BY ord`, owner)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item Redemption
		if err := rows.Scan(&item.ID, &item.Name, &item.Cost, &item.Date); err != nil {
			rows.Close()
			return nil, err
		}
		state.Redemptions = append(state.Redemptions, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	prefs := Prefs{}
	var prefsFound bool
	err = q.QueryRow(`SELECT name, goal, minutes, theme FROM prefs WHERE owner_id = ?`, owner).
		Scan(&prefs.Name, &prefs.Goal, &prefs.Minutes, &prefs.Theme)
	switch {
	case err == nil:
		prefsFound = true
		state.Prefs = prefs
	case err != sql.ErrNoRows:
		return nil, err
	}

	var timer Timer
	var timerEnd sql.NullFloat64
	timerFound := false
	err = q.QueryRow(`SELECT running, remaining, duration, "end" FROM timers WHERE owner_id = ?`, owner).
		Scan(&timer.Running, &timer.Remaining, &timer.Duration, &timerEnd)
	switch {
	case err == nil:
		timerFound = true
		if timerEnd.Valid {
			value := timerEnd.Float64
			timer.End = &value
		}
		state.Timer = &timer
	case err != sql.ErrNoRows:
		return nil, err
	}

	var config AIConfig
	var configProvider, configBase, configVision, configText sql.NullString
	var configDeepSeekBase, configDeepSeekVision, configDeepSeekText sql.NullString
	configFound := false
	err = q.QueryRow(`SELECT provider, base, vision, text, deepseek_base, deepseek_vision,
		deepseek_text FROM ai_config WHERE owner_id = ?`, owner).
		Scan(&configProvider, &configBase, &configVision, &configText,
			&configDeepSeekBase, &configDeepSeekVision, &configDeepSeekText)
	switch {
	case err == nil:
		configFound = true
		config.Provider = nullString(&configProvider)
		config.Base = nullString(&configBase)
		config.Vision = nullString(&configVision)
		config.Text = nullString(&configText)
		config.DeepSeekBase = nullString(&configDeepSeekBase)
		config.DeepSeekVision = nullString(&configDeepSeekVision)
		config.DeepSeekText = nullString(&configDeepSeekText)
		state.AIConfig = &config
	case err != sql.ErrNoRows:
		return nil, err
	}

	rows, err = q.Query(`SELECT id, ai_reviewed, source_id, source_title, subject, created
		FROM ai_sets WHERE owner_id = ? ORDER BY ord`, owner)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item AISet
		item.Questions = []AISetQuestion{}
		if err := rows.Scan(&item.ID, &item.AIReviewed, &item.SourceID, &item.SourceTitle,
			&item.Subject, &item.Created); err != nil {
			rows.Close()
			return nil, err
		}
		state.AISets = append(state.AISets, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	questionsBySet, err := readSetQuestions(q, owner)
	if err != nil {
		return nil, err
	}
	for index := range state.AISets {
		if items, ok := questionsBySet[state.AISets[index].ID]; ok {
			state.AISets[index].Questions = items
		}
	}

	state.Initialized = prefsFound || configFound || timerFound ||
		len(state.Questions) > 0 || len(state.Trash) > 0 || len(state.Logs) > 0 ||
		len(state.Rewards) > 0 || len(state.Redemptions) > 0 || len(state.AISets) > 0
	return state, nil
}

func nullString(value *sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

// readSetQuestions loads every AI-set question belonging to the owner in one
// query and groups them by set id, so reading the state costs a constant number
// of queries instead of one per set.
func readSetQuestions(q Querier, owner string) (map[string][]AISetQuestion, error) {
	rows, err := q.Query(`SELECT sq.set_id, sq.id, sq.title, sq.answer, sq.explanation,
		sq.knowledge, sq.note, sq.revealed, sq.grade
		FROM ai_set_questions sq JOIN ai_sets s ON s.id = sq.set_id
		WHERE s.owner_id = ? ORDER BY sq.set_id, sq.ord`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bySet := map[string][]AISetQuestion{}
	for rows.Next() {
		var setID string
		var item AISetQuestion
		if err := rows.Scan(&setID, &item.ID, &item.Title, &item.Answer, &item.Explanation,
			&item.Knowledge, &item.Note, &item.Revealed, &item.Grade); err != nil {
			return nil, err
		}
		bySet[setID] = append(bySet[setID], item)
	}
	return bySet, rows.Err()
}

// ------------------------------------------------------------------ validation

func text(value any, field string, limit int) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", invalid("%s 必须是文本", field)
	}
	if utf8.RuneCountInString(text) > limit {
		return "", invalid("%s 超过长度上限 %d", field, limit)
	}
	return text, nil
}

func ident(value any, field string) (string, error) {
	text, ok := value.(string)
	if !ok || !idPattern.MatchString(text) {
		return "", invalid("%s 格式不正确", field)
	}
	return text, nil
}

func date(value any, field string) (string, error) {
	text, ok := value.(string)
	if !ok || !datePattern.MatchString(text) {
		return "", invalid("%s 必须是 YYYY-MM-DD", field)
	}
	return text, nil
}

func boolean(value any, field string) (bool, error) {
	flag, ok := value.(bool)
	if !ok {
		return false, invalid("%s 必须是布尔值", field)
	}
	return flag, nil
}

func integer(value any, field string, low, high int) (int, error) {
	var number int64
	switch typed := value.(type) {
	case json.Number:
		literal := typed.String()
		if !intPattern.MatchString(literal) {
			return 0, invalid("%s 必须是 %d 到 %d 之间的整数", field, low, high)
		}
		parsed, err := strconv.ParseInt(literal, 10, 64)
		if err != nil {
			return 0, invalid("%s 必须是 %d 到 %d 之间的整数", field, low, high)
		}
		number = parsed
	case int:
		number = int64(typed)
	case int64:
		number = typed
	default:
		return 0, invalid("%s 必须是 %d 到 %d 之间的整数", field, low, high)
	}
	if number < int64(low) || number > int64(high) {
		return 0, invalid("%s 必须是 %d 到 %d 之间的整数", field, low, high)
	}
	return int(number), nil
}

func number(value any, field string, low, high float64) (float64, error) {
	var parsed float64
	switch typed := value.(type) {
	case json.Number:
		converted, err := typed.Float64()
		if err != nil {
			return 0, invalid("%s 超出允许范围", field)
		}
		parsed = converted
	case int:
		parsed = float64(typed)
	case int64:
		parsed = float64(typed)
	case float64:
		parsed = typed
	default:
		return 0, invalid("%s 超出允许范围", field)
	}
	if parsed < low || parsed > high {
		return 0, invalid("%s 超出允许范围", field)
	}
	return parsed, nil
}

func fieldValue(row map[string]any, name string, fallback any) any {
	if value, ok := row[name]; ok {
		return value
	}
	return fallback
}

func textField(row map[string]any, name, fallback string, limit int) (string, error) {
	return text(fieldValue(row, name, fallback), name, limit)
}

func boolField(row map[string]any, name string, fallback bool) (bool, error) {
	return boolean(fieldValue(row, name, fallback), name)
}

func intField(row map[string]any, name string, fallback, low, high int) (int, error) {
	return integer(fieldValue(row, name, fallback), name, low, high)
}

func numberField(row map[string]any, name string, fallback, low, high float64) (float64, error) {
	return number(fieldValue(row, name, fallback), name, low, high)
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return typed != ""
	case json.Number:
		parsed, err := typed.Float64()
		return err == nil && parsed != 0
	case float64:
		return typed != 0
	case int:
		return typed != 0
	case int64:
		return typed != 0
	default:
		return true
	}
}

func rowOf(op map[string]any) map[string]any {
	value := op["row"]
	if value == nil {
		return map[string]any{}
	}
	if row, ok := value.(map[string]any); ok {
		return row
	}
	return nil
}

func nullableString(value any, field string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, invalid("%s 必须是文本", field)
	}
	return &text, nil
}

// ------------------------------------------------------------------ write ops

func upsertQuestion(tx *sql.Tx, owner string, op map[string]any) error {
	row := rowOf(op)
	if row == nil {
		return invalid("操作格式不正确")
	}
	idValue := op["id"]
	if !truthy(idValue) {
		idValue = row["id"]
	}
	id, err := ident(idValue, "id")
	if err != nil {
		return err
	}
	values := make([]any, 0, 16)
	for _, field := range []string{"subject", "title", "answer", "reason", "tag", "cause", "difficulty"} {
		value, err := textField(row, field, "", MaxText)
		if err != nil {
			return err
		}
		values = append(values, value)
	}
	image, err := textField(row, "image", "", MaxImage)
	if err != nil {
		return err
	}
	favorite, err := boolField(row, "favorite", false)
	if err != nil {
		return err
	}
	demo, err := boolField(row, "demo", false)
	if err != nil {
		return err
	}
	stage, err := intField(row, "stage", 0, 0, 5)
	if err != nil {
		return err
	}
	due, err := date(fieldValue(row, "due", "1970-01-01"), "due")
	if err != nil {
		return err
	}
	created, err := date(fieldValue(row, "created", "1970-01-01"), "created")
	if err != nil {
		return err
	}
	trashed, err := boolField(op, "trashed", false)
	if err != nil {
		return err
	}
	ord, err := intField(op, "ord", 0, 0, MaxOrd)
	if err != nil {
		return err
	}
	if err := checkOwner(tx, "questions", id, owner); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO questions (id, owner_id, ord, trashed, subject, title,
		answer, reason, tag, cause, difficulty, image, favorite, stage, due, created, demo)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET ord=excluded.ord, trashed=excluded.trashed,
		subject=excluded.subject, title=excluded.title, answer=excluded.answer,
		reason=excluded.reason, tag=excluded.tag, cause=excluded.cause,
		difficulty=excluded.difficulty, image=excluded.image, favorite=excluded.favorite,
		stage=excluded.stage, due=excluded.due, created=excluded.created, demo=excluded.demo`,
		append([]any{id, owner, ord, trashed}, append(values[:7], image,
			favorite, stage, due, created, demo)...)...)
	if err != nil {
		return err
	}
	return nil
}

func upsertLog(tx *sql.Tx, owner string, op map[string]any) error {
	row := rowOf(op)
	if row == nil {
		return invalid("操作格式不正确")
	}
	id, err := ident(row["id"], "id")
	if err != nil {
		return err
	}
	dateValue, err := date(row["date"], "date")
	if err != nil {
		return err
	}
	at, err := textField(row, "at", "", 40)
	if err != nil {
		return err
	}
	kind, err := textField(row, "type", "", 20)
	if err != nil {
		return err
	}
	label, err := textField(row, "label", "", MaxText)
	if err != nil {
		return err
	}
	points, err := numberField(row, "points", 0, 0, 30)
	if err != nil {
		return err
	}
	if kind != "add" && kind != "review" && kind != "practice" && kind != "focus" {
		return invalid("type 只能是 add/review/practice/focus")
	}
	qid, err := nullableString(row["qid"], "qid")
	if err != nil {
		return err
	}
	grade, err := nullableString(row["grade"], "grade")
	if err != nil {
		return err
	}
	if (kind == "review" || kind == "practice") &&
		(grade == nil || (*grade != "again" && *grade != "hard" && *grade != "good")) {
		return invalid("复习记录必须带有自评结果")
	}
	var minutes *float64
	if value, ok := row["minutes"]; ok && value != nil {
		parsed, err := number(value, "minutes", 0, 24*60)
		if err != nil {
			return err
		}
		minutes = &parsed
	}
	ord, err := intField(op, "ord", 0, 0, MaxOrd)
	if err != nil {
		return err
	}
	if err := checkOwner(tx, "logs", id, owner); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO logs (id, owner_id, ord, date, at, type, label, points,
		qid, grade, minutes) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET ord=excluded.ord, date=excluded.date, at=excluded.at,
		type=excluded.type, label=excluded.label, points=excluded.points, qid=excluded.qid,
		grade=excluded.grade, minutes=excluded.minutes`,
		id, owner, ord, dateValue, at, kind, label, points, qid, grade, minutes)
	return err
}

func upsertNamed(tx *sql.Tx, owner string, op map[string]any, table string, withDate bool) error {
	row := rowOf(op)
	if row == nil {
		return invalid("操作格式不正确")
	}
	id, err := ident(row["id"], "id")
	if err != nil {
		return err
	}
	name, err := textField(row, "name", "", MaxText)
	if err != nil {
		return err
	}
	cost, err := intField(row, "cost", 0, 0, 10_000)
	if err != nil {
		return err
	}
	ord, err := intField(op, "ord", 0, 0, MaxOrd)
	if err != nil {
		return err
	}
	if err := checkOwner(tx, table, id, owner); err != nil {
		return err
	}
	if withDate {
		dateValue, err := date(row["date"], "date")
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO redemptions (id, owner_id, ord, name, cost, date)
			VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET ord=excluded.ord,
			name=excluded.name, cost=excluded.cost, date=excluded.date`,
			id, owner, ord, name, cost, dateValue)
		return err
	}
	_, err = tx.Exec(`INSERT INTO rewards (id, owner_id, ord, name, cost)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET ord=excluded.ord,
		name=excluded.name, cost=excluded.cost`, id, owner, ord, name, cost)
	return err
}

func upsertAISet(tx *sql.Tx, owner string, op map[string]any) error {
	row := rowOf(op)
	if row == nil {
		return invalid("操作格式不正确")
	}
	id, err := ident(row["id"], "id")
	if err != nil {
		return err
	}
	questions, ok := row["questions"].([]any)
	if !ok || len(questions) != 3 {
		return invalid("AI 练习组必须恰好包含三道题")
	}
	sourceID, err := textField(row, "sourceId", "", 100)
	if err != nil {
		return err
	}
	sourceTitle, err := textField(row, "sourceTitle", "", MaxText)
	if err != nil {
		return err
	}
	subject, err := textField(row, "subject", "", MaxText)
	if err != nil {
		return err
	}
	created, err := date(row["created"], "created")
	if err != nil {
		return err
	}
	reviewed, err := boolField(row, "aiReviewed", false)
	if err != nil {
		return err
	}
	ord, err := intField(op, "ord", 0, 0, MaxOrd)
	if err != nil {
		return err
	}
	if err := checkOwner(tx, "ai_sets", id, owner); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO ai_sets (id, owner_id, ord, source_id, source_title,
		subject, created, ai_reviewed) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET ord=excluded.ord, source_id=excluded.source_id,
		source_title=excluded.source_title, subject=excluded.subject,
		created=excluded.created, ai_reviewed=excluded.ai_reviewed`,
		id, owner, ord, sourceID, sourceTitle, subject, created, reviewed); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ai_set_questions WHERE set_id = ?`, id); err != nil {
		return err
	}
	for index, raw := range questions {
		item, ok := raw.(map[string]any)
		if !ok {
			return invalid("练习题格式不正确")
		}
		itemID, err := ident(item["id"], "id")
		if err != nil {
			return err
		}
		title, err := textField(item, "title", "", MaxText)
		if err != nil {
			return err
		}
		answer, err := textField(item, "answer", "", MaxText)
		if err != nil {
			return err
		}
		explanation, err := textField(item, "explanation", "", MaxText)
		if err != nil {
			return err
		}
		knowledge, err := textField(item, "knowledge", "", 200)
		if err != nil {
			return err
		}
		note, err := textField(item, "note", "", 5_000)
		if err != nil {
			return err
		}
		revealed, err := boolField(item, "revealed", false)
		if err != nil {
			return err
		}
		grade, err := textField(item, "grade", "", 20)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO ai_set_questions (id, set_id, ord, title, answer,
			explanation, knowledge, note, revealed, grade)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			itemID, id, index, title, answer, explanation, knowledge, note, revealed, grade); err != nil {
			return err
		}
	}
	return nil
}

func replacePrefs(tx *sql.Tx, owner string, op map[string]any) error {
	row := rowOf(op)
	if row == nil {
		return invalid("操作格式不正确")
	}
	name, err := textField(row, "name", "", 20)
	if err != nil {
		return err
	}
	goal, err := intField(row, "goal", 5, 1, 100)
	if err != nil {
		return err
	}
	minutes, err := intField(row, "minutes", 25, 1, 120)
	if err != nil {
		return err
	}
	theme, err := textField(row, "theme", "light", 20)
	if err != nil {
		return err
	}
	if theme != "light" && theme != "night" {
		return invalid("theme 只能是 light 或 night")
	}
	_, err = tx.Exec(`INSERT INTO prefs (owner_id, name, goal, minutes, theme)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(owner_id) DO UPDATE SET name=excluded.name,
		goal=excluded.goal, minutes=excluded.minutes, theme=excluded.theme`,
		owner, name, goal, minutes, theme)
	return err
}

func replaceTimer(tx *sql.Tx, owner string, op map[string]any) error {
	value := op["row"]
	if value == nil {
		_, err := tx.Exec(`DELETE FROM timers WHERE owner_id = ?`, owner)
		return err
	}
	row, ok := value.(map[string]any)
	if !ok {
		return invalid("计时数据格式不正确")
	}
	running, err := boolField(row, "running", false)
	if err != nil {
		return err
	}
	duration, err := numberField(row, "duration", 0, 0, 24*3600)
	if err != nil {
		return err
	}
	remaining, err := numberField(row, "remaining", 0, 0, 24*3600)
	if err != nil {
		return err
	}
	var end *float64
	if raw, ok := row["end"]; ok && raw != nil {
		parsed, err := number(raw, "end", 0, 4_102_444_800_000)
		if err != nil {
			return err
		}
		end = &parsed
	}
	if remaining > duration {
		return invalid("剩余时间不能超过总时长")
	}
	_, err = tx.Exec(`INSERT INTO timers (owner_id, running, remaining, duration, "end")
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(owner_id) DO UPDATE SET running=excluded.running,
		remaining=excluded.remaining, duration=excluded.duration, "end"=excluded."end"`,
		owner, running, remaining, duration, end)
	return err
}

func replaceAIConfig(tx *sql.Tx, owner string, op map[string]any) error {
	value := op["row"]
	if value == nil {
		_, err := tx.Exec(`DELETE FROM ai_config WHERE owner_id = ?`, owner)
		return err
	}
	row, ok := value.(map[string]any)
	if !ok {
		return invalid("AI 配置格式不正确")
	}
	provider, err := textField(row, "provider", "qwen", 20)
	if err != nil {
		return err
	}
	if provider != "qwen" && provider != "deepseek" {
		return invalid("provider 只能是 qwen 或 deepseek")
	}
	base, err := textField(row, "base", "", 500)
	if err != nil {
		return err
	}
	vision, err := textField(row, "vision", "", 120)
	if err != nil {
		return err
	}
	textModel, err := textField(row, "text", "", 120)
	if err != nil {
		return err
	}
	deepSeekBase, err := textField(row, "deepseekBase", "", 500)
	if err != nil {
		return err
	}
	deepSeekVision, err := textField(row, "deepseekVision", "", 120)
	if err != nil {
		return err
	}
	deepSeekText, err := textField(row, "deepseekText", "", 120)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO ai_config (owner_id, provider, base, vision, text,
		deepseek_base, deepseek_vision, deepseek_text) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(owner_id) DO UPDATE SET provider=excluded.provider, base=excluded.base,
		vision=excluded.vision, text=excluded.text, deepseek_base=excluded.deepseek_base,
		deepseek_vision=excluded.deepseek_vision, deepseek_text=excluded.deepseek_text`,
		owner, provider, base, vision, textModel, deepSeekBase, deepSeekVision, deepSeekText)
	return err
}

func checkOwner(tx *sql.Tx, table, id, owner string) error {
	var existing string
	err := tx.QueryRow(`SELECT owner_id FROM `+table+` WHERE id = ?`, id).Scan(&existing)
	switch {
	case err == nil:
		if existing != owner {
			return invalid("记录编号已被使用")
		}
		return nil
	case err == sql.ErrNoRows:
		return nil
	default:
		return err
	}
}

func deleteByID(tx *sql.Tx, table, owner string, op map[string]any) error {
	id, err := ident(op["id"], "id")
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM `+table+` WHERE id = ? AND owner_id = ?`, id, owner)
	return err
}

// ApplyOps validates and applies one batch. The caller owns the transaction.
func ApplyOps(tx *sql.Tx, owner string, operations []any) (int, error) {
	if len(operations) > MaxOps {
		return 0, invalid("单次操作过多")
	}
	for _, raw := range operations {
		op, ok := raw.(map[string]any)
		if !ok {
			return 0, invalid("操作格式不正确")
		}
		kind, _ := op["t"].(string)
		action, _ := op["op"].(string)
		if kind == "reset" && action == "reset" {
			if err := clearOwner(tx, owner); err != nil {
				return 0, err
			}
			continue
		}
		switch kind {
		case "questions":
			switch action {
			case "upsert":
				if err := upsertQuestion(tx, owner, op); err != nil {
					return 0, err
				}
			case "delete":
				if err := deleteByID(tx, "questions", owner, op); err != nil {
					return 0, err
				}
			default:
				return 0, invalid("未知的 questions 操作")
			}
		case "logs":
			switch action {
			case "upsert", "append":
				if err := upsertLog(tx, owner, op); err != nil {
					return 0, err
				}
			case "delete":
				if err := deleteByID(tx, "logs", owner, op); err != nil {
					return 0, err
				}
			default:
				return 0, invalid("未知的 logs 操作")
			}
		case "rewards":
			switch action {
			case "upsert":
				if err := upsertNamed(tx, owner, op, "rewards", false); err != nil {
					return 0, err
				}
			case "delete":
				if err := deleteByID(tx, "rewards", owner, op); err != nil {
					return 0, err
				}
			default:
				return 0, invalid("未知的 rewards 操作")
			}
		case "redemptions":
			switch action {
			case "upsert":
				if err := upsertNamed(tx, owner, op, "redemptions", true); err != nil {
					return 0, err
				}
			case "delete":
				if err := deleteByID(tx, "redemptions", owner, op); err != nil {
					return 0, err
				}
			default:
				return 0, invalid("未知的 redemptions 操作")
			}
		case "prefs":
			if action != "replace" {
				return 0, invalid("未知的 prefs 操作")
			}
			if err := replacePrefs(tx, owner, op); err != nil {
				return 0, err
			}
		case "timer":
			if action != "replace" {
				return 0, invalid("未知的 timer 操作")
			}
			if err := replaceTimer(tx, owner, op); err != nil {
				return 0, err
			}
		case "aiConfig":
			if action != "replace" {
				return 0, invalid("未知的 aiConfig 操作")
			}
			if err := replaceAIConfig(tx, owner, op); err != nil {
				return 0, err
			}
		case "aiSets":
			switch action {
			case "upsert":
				if err := upsertAISet(tx, owner, op); err != nil {
					return 0, err
				}
			case "delete":
				id, err := ident(op["id"], "id")
				if err != nil {
					return 0, err
				}
				var existing string
				err = tx.QueryRow(`SELECT owner_id FROM ai_sets WHERE id = ?`, id).Scan(&existing)
				if err == sql.ErrNoRows {
					break
				}
				if err != nil {
					return 0, err
				}
				if existing != owner {
					break
				}
				if _, err := tx.Exec(`DELETE FROM ai_set_questions WHERE set_id = ?`, id); err != nil {
					return 0, err
				}
				if _, err := tx.Exec(`DELETE FROM ai_sets WHERE id = ?`, id); err != nil {
					return 0, err
				}
			default:
				return 0, invalid("未知的 aiSets 操作")
			}
		default:
			return 0, invalid("未知的数据表 %v", op["t"])
		}
	}
	return len(operations), nil
}

// clearOwner deletes every learning row of one owner and keeps ai_keys.
func clearOwner(tx *sql.Tx, owner string) error {
	statements := []string{
		`DELETE FROM ai_set_questions WHERE set_id IN (SELECT id FROM ai_sets WHERE owner_id = ?)`,
		`DELETE FROM ai_sets WHERE owner_id = ?`,
		`DELETE FROM logs WHERE owner_id = ?`,
		`DELETE FROM rewards WHERE owner_id = ?`,
		`DELETE FROM redemptions WHERE owner_id = ?`,
		`DELETE FROM questions WHERE owner_id = ?`,
		`DELETE FROM prefs WHERE owner_id = ?`,
		`DELETE FROM timers WHERE owner_id = ?`,
		`DELETE FROM ai_config WHERE owner_id = ?`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement, owner); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceState clears one owner and re-imports a whole exported state.
func ReplaceState(tx *sql.Tx, owner string, state map[string]any) error {
	schema, ok := state["schema"]
	if !ok || !numberEquals(schema, 1) {
		return invalid("备份格式不正确")
	}
	questions, ok := state["questions"].([]any)
	if !ok {
		return invalid("备份格式不正确")
	}
	if _, ok := state["logs"].([]any); !ok {
		return invalid("备份格式不正确")
	}
	prefs, ok := state["prefs"].(map[string]any)
	if !ok {
		return invalid("备份格式不正确")
	}
	for _, field := range []string{"questions", "logs", "rewards", "redemptions", "aiSets", "trash"} {
		value, exists := state[field]
		if !exists {
			continue
		}
		list, ok := value.([]any)
		if !ok {
			return invalid("%s 备份数据格式不正确", field)
		}
		for _, item := range list {
			if _, ok := item.(map[string]any); !ok {
				return invalid("%s 备份数据格式不正确", field)
			}
		}
	}

	if err := clearOwner(tx, owner); err != nil {
		return err
	}
	operations := []any{}
	appendRows := func(kind string, items []any, trashed bool) {
		for index, item := range items {
			op := map[string]any{
				"t": kind, "op": "upsert", "id": item.(map[string]any)["id"],
				"ord": index, "row": item,
			}
			if kind == "questions" {
				op["trashed"] = trashed
			}
			operations = append(operations, op)
		}
	}
	appendRows("questions", questions, false)
	if trash, ok := state["trash"].([]any); ok {
		appendRows("questions", trash, true)
	}
	for _, field := range []string{"logs", "rewards", "redemptions", "aiSets"} {
		if items, ok := state[field].([]any); ok {
			appendRows(field, items, false)
		}
	}
	operations = append(operations, map[string]any{"t": "prefs", "op": "replace", "row": prefs})
	operations = append(operations, map[string]any{"t": "timer", "op": "replace", "row": state["timer"]})
	if config, ok := state["aiConfig"].(map[string]any); ok {
		operations = append(operations, map[string]any{"t": "aiConfig", "op": "replace", "row": config})
	}
	_, err := ApplyOps(tx, owner, operations)
	return err
}

func numberEquals(value any, target float64) bool {
	parsed, err := number(value, "schema", -1e18, 1e18)
	return err == nil && parsed == target
}

// ----------------------------------------------------------------------- keys

func SetKey(tx *sql.Tx, owner, provider, ciphertext string) error {
	updated := time.Now().UTC().Format("2006-01-02T15:04:05") + "+00:00"
	_, err := tx.Exec(`INSERT INTO ai_keys (owner_id, provider, ciphertext, updated)
		VALUES (?, ?, ?, ?) ON CONFLICT(owner_id, provider) DO UPDATE SET
		ciphertext=excluded.ciphertext, updated=excluded.updated`,
		owner, provider, ciphertext, updated)
	return err
}

func ReadKey(q Querier, owner, provider string) (string, error) {
	var ciphertext string
	err := q.QueryRow(`SELECT ciphertext FROM ai_keys WHERE owner_id = ? AND provider = ?`,
		owner, provider).Scan(&ciphertext)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return ciphertext, nil
}

func DeleteKey(tx *sql.Tx, owner, provider string) (bool, error) {
	result, err := tx.Exec(`DELETE FROM ai_keys WHERE owner_id = ? AND provider = ?`, owner, provider)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func ReadKeys(q Querier, owner string) (map[string]string, error) {
	rows, err := q.Query(`SELECT provider, ciphertext FROM ai_keys WHERE owner_id = ?`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := map[string]string{}
	for rows.Next() {
		var provider, ciphertext string
		if err := rows.Scan(&provider, &ciphertext); err != nil {
			return nil, err
		}
		keys[provider] = ciphertext
	}
	return keys, rows.Err()
}
