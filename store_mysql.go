package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

type mysqlStore struct{ db *sql.DB }

func newMySQLStore(c *Config, waitAttempts int) (*mysqlStore, error) {
	db, err := openDB(c, waitAttempts)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &mysqlStore{db: db}, nil
}

func (m *mysqlStore) Kind() string                   { return "mysql" }
func (m *mysqlStore) Ping(ctx context.Context) error { return m.db.PingContext(ctx) }
func (m *mysqlStore) Close() error                   { return m.db.Close() }

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullRaw(r json.RawMessage) any {
	if len(r) == 0 || string(r) == "null" {
		return nil
	}
	return string(r)
}

// ---------- Sitzungen ----------

func (m *mysqlStore) CreateSession(ctx context.Context, s *SessionRec) error {
	_, err := m.db.ExecContext(ctx, `INSERT INTO sessions (id_hash, username, display_name, is_admin, auth_source, csrf, created_at, last_seen, expires_at, ip, user_agent) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		s.IDHash, s.Username, s.DisplayName, s.IsAdmin, s.Source, s.CSRF, s.CreatedAt, s.LastSeen, s.ExpiresAt, s.IP, s.UserAgent)
	return err
}

func (m *mysqlStore) GetSession(ctx context.Context, h string) (*SessionRec, error) {
	s := &SessionRec{IDHash: h}
	err := m.db.QueryRowContext(ctx, `SELECT username, display_name, is_admin, auth_source, csrf, created_at, last_seen, expires_at FROM sessions WHERE id_hash = ?`, h).
		Scan(&s.Username, &s.DisplayName, &s.IsAdmin, &s.Source, &s.CSRF, &s.CreatedAt, &s.LastSeen, &s.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

func (m *mysqlStore) TouchSession(ctx context.Context, h string, t time.Time) error {
	_, err := m.db.ExecContext(ctx, `UPDATE sessions SET last_seen = ? WHERE id_hash = ?`, t, h)
	return err
}

func (m *mysqlStore) DeleteSession(ctx context.Context, h string) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, h)
	return err
}

func (m *mysqlStore) DeleteUserSessions(ctx context.Context, user, source string) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM sessions WHERE username = ? AND auth_source = ?`, user, source)
	return err
}

func (m *mysqlStore) PurgeSessions(ctx context.Context, expired, idle time.Time) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ? OR last_seen < ?`, expired, idle)
	return err
}

// ---------- Lokale Konten ----------

func (m *mysqlStore) GetLocalUser(ctx context.Context, user string) (*LocalUser, error) {
	u := &LocalUser{Username: user}
	var ll sql.NullTime
	err := m.db.QueryRowContext(ctx, `SELECT display_name, pw_hash, is_admin, disabled, created_at, last_login FROM local_users WHERE username = ?`, user).
		Scan(&u.DisplayName, &u.PwHash, &u.IsAdmin, &u.Disabled, &u.CreatedAt, &ll)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if ll.Valid {
		u.LastLogin = &ll.Time
	}
	return u, err
}

func (m *mysqlStore) UpsertLocalUser(ctx context.Context, u LocalUser) error {
	_, err := m.db.ExecContext(ctx, `INSERT INTO local_users (username, display_name, pw_hash, is_admin, disabled, created_at) VALUES (?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE display_name = VALUES(display_name), pw_hash = VALUES(pw_hash), is_admin = VALUES(is_admin), disabled = VALUES(disabled)`,
		u.Username, u.DisplayName, u.PwHash, u.IsAdmin, u.Disabled, u.CreatedAt)
	return err
}

func (m *mysqlStore) TouchLocalUser(ctx context.Context, user string, t time.Time) error {
	_, err := m.db.ExecContext(ctx, `UPDATE local_users SET last_login = ? WHERE username = ?`, t, user)
	return err
}

// ---------- Einträge ----------

const empCols = `id, COALESCE(username,''), last_name, first_name, phone, mobile, room, amt, email, source, COALESCE(ad_guid,''), COALESCE(ad_dn,''), active, hidden, COALESCE(avatar,''), created_at, updated_at, last_synced_at, COALESCE(org_unit_id,0), COALESCE(deputies,''), COALESCE(absences,''), COALESCE(channel_id,0)`

// jsonCol: Liste als JSON-Text (NULL, wenn leer)
func jsonCol[T any](v []T) any {
	if len(v) == 0 {
		return nil
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func parseCol[T any](s string) []T {
	if s == "" {
		return nil
	}
	var v []T
	if json.Unmarshal([]byte(s), &v) != nil || len(v) == 0 {
		return nil
	}
	return v
}

type scanner interface{ Scan(...any) error }

func scanEmp(r scanner) (EmployeeRec, error) {
	var e EmployeeRec
	var av, deps, abs string
	var ls sql.NullTime
	err := r.Scan(&e.ID, &e.Username, &e.LastName, &e.FirstName, &e.Phone, &e.Mobile, &e.Room, &e.Amt, &e.Email, &e.Source, &e.ADGUID, &e.ADDN, &e.Active, &e.Hidden, &av, &e.CreatedAt, &e.UpdatedAt, &ls, &e.OrgUnit, &deps, &abs, &e.Channel)
	e.Deputies = parseCol[int64](deps)
	e.Absences = parseCol[Absence](abs)
	if av != "" {
		e.Avatar = json.RawMessage(av)
	}
	if ls.Valid {
		e.LastSyncedAt = &ls.Time
	}
	e.Tags = []int64{}
	return e, err
}

func (m *mysqlStore) ListEmployees(ctx context.Context, all bool) ([]EmployeeRec, error) {
	q := `SELECT ` + empCols + ` FROM employees`
	if !all {
		q += ` WHERE active = 1 AND hidden = 0`
	}
	rows, err := m.db.QueryContext(ctx, q+` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	out := []EmployeeRec{}
	idx := map[int64]int{}
	for rows.Next() {
		e, err := scanEmp(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		idx[e.ID] = len(out)
		out = append(out, e)
	}
	rows.Close()
	rows, err = m.db.QueryContext(ctx, `SELECT employee_id, tag_id FROM employee_tags ORDER BY employee_id, tag_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var eid, tid int64
		if err := rows.Scan(&eid, &tid); err != nil {
			return nil, err
		}
		if i, ok := idx[eid]; ok {
			out[i].Tags = append(out[i].Tags, tid)
		}
	}
	return out, rows.Err()
}

func (m *mysqlStore) GetEmployee(ctx context.Context, id int64) (*EmployeeRec, error) {
	e, err := scanEmp(m.db.QueryRowContext(ctx, `SELECT `+empCols+` FROM employees WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := m.db.QueryContext(ctx, `SELECT tag_id FROM employee_tags WHERE employee_id = ? ORDER BY tag_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t int64
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		e.Tags = append(e.Tags, t)
	}
	return &e, rows.Err()
}

func (m *mysqlStore) EmployeeIDByUsername(ctx context.Context, user string) (int64, error) {
	var id int64
	err := m.db.QueryRowContext(ctx, `SELECT id FROM employees WHERE username = ? LIMIT 1`, user).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (m *mysqlStore) CreateEmployee(ctx context.Context, e *EmployeeRec) (int64, error) {
	res, err := m.db.ExecContext(ctx, `INSERT INTO employees (username, last_name, first_name, phone, mobile, room, amt, email, source, ad_guid, ad_dn, active, hidden, avatar, created_at, updated_at, last_synced_at, org_unit_id, deputies, absences, channel_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		nullStr(e.Username), e.LastName, e.FirstName, e.Phone, e.Mobile, e.Room, e.Amt, e.Email, e.Source, nullStr(e.ADGUID), nullStr(e.ADDN), e.Active, e.Hidden, nullRaw(e.Avatar), e.CreatedAt, e.UpdatedAt, e.LastSyncedAt, nullID(e.OrgUnit), jsonCol(e.Deputies), jsonCol(e.Absences), nullID(e.Channel))
	if isDuplicate(err) {
		return 0, ErrDuplicate
	}
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *mysqlStore) UpdateEmployee(ctx context.Context, e *EmployeeRec) error {
	res, err := m.db.ExecContext(ctx, `UPDATE employees SET username = ?, last_name = ?, first_name = ?, phone = ?, mobile = ?, room = ?, amt = ?, email = ?, hidden = ?, updated_at = ? WHERE id = ?`,
		nullStr(e.Username), e.LastName, e.FirstName, e.Phone, e.Mobile, e.Room, e.Amt, e.Email, e.Hidden, e.UpdatedAt, e.ID)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	return m.mustExist(ctx, res, `SELECT 1 FROM employees WHERE id = ?`, e.ID)
}

// RowsAffected = 0 kann auch "keine Änderung" heißen – dann Existenz prüfen
func (m *mysqlStore) mustExist(ctx context.Context, res sql.Result, q string, args ...any) error {
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	var x int
	if err := m.db.QueryRowContext(ctx, q, args...).Scan(&x); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	return nil
}

func (m *mysqlStore) SetAvatar(ctx context.Context, id int64, av json.RawMessage) error {
	res, err := m.db.ExecContext(ctx, `UPDATE employees SET avatar = ?, updated_at = ? WHERE id = ?`, nullRaw(av), time.Now().UTC(), id)
	if err != nil {
		return err
	}
	return m.mustExist(ctx, res, `SELECT 1 FROM employees WHERE id = ?`, id)
}

func (m *mysqlStore) DeleteEmployee(ctx context.Context, id int64) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM employee_tags WHERE employee_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM private_shares WHERE kind = 'emp' AND value = ?`, strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM employees WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reports SET employee_id = NULL WHERE employee_id = ?`, id); err != nil {
		return err
	}
	if err := removeRefsTx(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *mysqlStore) LinkUsernameByGUID(ctx context.Context, guid, user string) (bool, error) {
	res, err := m.db.ExecContext(ctx, `UPDATE employees SET username = ? WHERE ad_guid = ? AND username IS NULL`, user, guid)
	if isDuplicate(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (m *mysqlStore) SetEmployeeTags(ctx context.Context, id int64, tags []TagInput, creator string, allowCreate bool) ([]int64, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var x int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM employees WHERE id = ?`, id).Scan(&x); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	ids := []int64{}
	seen := map[int64]bool{}
	for _, t := range tags {
		var tid int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE LOWER(name) = LOWER(?) ORDER BY id LIMIT 1`, t.Name).Scan(&tid)
		if errors.Is(err, sql.ErrNoRows) {
			if !allowCreate {
				return nil, ErrUnknownTag{t.Name}
			}
			res, err := tx.ExecContext(ctx, `INSERT INTO tags (name, category, created_by, created_at) VALUES (?,?,?,?)`, t.Name, t.Category, creator, now)
			if err != nil {
				return nil, err
			}
			tid, _ = res.LastInsertId()
		} else if err != nil {
			return nil, err
		}
		if !seen[tid] {
			seen[tid] = true
			ids = append(ids, tid)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM employee_tags WHERE employee_id = ?`, id); err != nil {
		return nil, err
	}
	for _, tid := range ids {
		if _, err := tx.ExecContext(ctx, `INSERT INTO employee_tags (employee_id, tag_id, created_at) VALUES (?,?,?)`, id, tid, now); err != nil {
			return nil, err
		}
	}
	return ids, tx.Commit()
}

// ---------- Schlagworte ----------

func (m *mysqlStore) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT t.id, t.name, t.category, t.created_by, (SELECT COUNT(*) FROM employee_tags et WHERE et.tag_id = t.id) FROM tags t ORDER BY t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Category, &t.CreatedBy, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (m *mysqlStore) tagTaken(ctx context.Context, name string, except int64) (bool, error) {
	var id int64
	err := m.db.QueryRowContext(ctx, `SELECT id FROM tags WHERE LOWER(name) = LOWER(?) AND id <> ? LIMIT 1`, name, except).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (m *mysqlStore) CreateTag(ctx context.Context, name, cat, creator string) (int64, error) {
	if taken, err := m.tagTaken(ctx, name, 0); err != nil {
		return 0, err
	} else if taken {
		return 0, ErrDuplicate
	}
	res, err := m.db.ExecContext(ctx, `INSERT INTO tags (name, category, created_by, created_at) VALUES (?,?,?,?)`, name, cat, creator, time.Now().UTC())
	if isDuplicate(err) {
		return 0, ErrDuplicate
	}
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *mysqlStore) tagName(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64) (string, error) {
	var n string
	err := q.QueryRowContext(ctx, `SELECT name FROM tags WHERE id = ?`, id).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return n, err
}

func (m *mysqlStore) UpdateTag(ctx context.Context, id int64, name, cat string) (string, error) {
	old, err := m.tagName(ctx, m.db, id)
	if err != nil {
		return "", err
	}
	if taken, err := m.tagTaken(ctx, name, id); err != nil {
		return "", err
	} else if taken {
		return "", ErrDuplicate
	}
	_, err = m.db.ExecContext(ctx, `UPDATE tags SET name = ?, category = ? WHERE id = ?`, name, cat, id)
	if isDuplicate(err) {
		return "", ErrDuplicate
	}
	return old, err
}

func (m *mysqlStore) MergeTag(ctx context.Context, from, into int64) (string, string, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	fn, err := m.tagName(ctx, tx, from)
	if err != nil {
		return "", "", err
	}
	in, err := m.tagName(ctx, tx, into)
	if err != nil {
		return "", "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO employee_tags (employee_id, tag_id, created_at)
		SELECT et.employee_id, ?, et.created_at FROM employee_tags et
		WHERE et.tag_id = ? AND NOT EXISTS (SELECT 1 FROM employee_tags x WHERE x.employee_id = et.employee_id AND x.tag_id = ?)`, into, from, into); err != nil {
		return "", "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM employee_tags WHERE tag_id = ?`, from); err != nil {
		return "", "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, from); err != nil {
		return "", "", err
	}
	return fn, in, tx.Commit()
}

func (m *mysqlStore) DeleteTag(ctx context.Context, id int64) (string, error) {
	n, err := m.tagName(ctx, m.db, id)
	if err != nil {
		return "", err
	}
	if _, err := m.db.ExecContext(ctx, `DELETE FROM employee_tags WHERE tag_id = ?`, id); err != nil {
		return "", err
	}
	_, err = m.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	return n, err
}

func (m *mysqlStore) DeleteUnusedTags(ctx context.Context) (int64, error) {
	res, err := m.db.ExecContext(ctx, `DELETE FROM tags WHERE NOT EXISTS (SELECT 1 FROM employee_tags et WHERE et.tag_id = tags.id)`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---------- Kategorien ----------

func (m *mysqlStore) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT c.cat_key, c.name, c.color, c.sort, (SELECT COUNT(*) FROM tags t WHERE t.category = c.cat_key) FROM tag_categories c ORDER BY c.sort, c.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.Key, &c.Name, &c.Color, &c.Sort, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (m *mysqlStore) catNameTaken(ctx context.Context, name, exceptKey string) (bool, error) {
	var k string
	err := m.db.QueryRowContext(ctx, `SELECT cat_key FROM tag_categories WHERE LOWER(name) = LOWER(?) AND cat_key <> ? LIMIT 1`, name, exceptKey).Scan(&k)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (m *mysqlStore) CreateCategory(ctx context.Context, c Category) error {
	if taken, err := m.catNameTaken(ctx, c.Name, ""); err != nil {
		return err
	} else if taken {
		return ErrDuplicate
	}
	_, err := m.db.ExecContext(ctx, `INSERT INTO tag_categories (cat_key, name, color, sort, created_at) VALUES (?,?,?,?,?)`, c.Key, c.Name, c.Color, c.Sort, time.Now().UTC())
	if isDuplicate(err) {
		return ErrDuplicate
	}
	return err
}

func (m *mysqlStore) UpdateCategory(ctx context.Context, c Category) error {
	if taken, err := m.catNameTaken(ctx, c.Name, c.Key); err != nil {
		return err
	} else if taken {
		return ErrDuplicate
	}
	res, err := m.db.ExecContext(ctx, `UPDATE tag_categories SET name = ?, color = ?, sort = ? WHERE cat_key = ?`, c.Name, c.Color, c.Sort, c.Key)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	return m.mustExist(ctx, res, `SELECT 1 FROM tag_categories WHERE cat_key = ?`, c.Key)
}

func (m *mysqlStore) DeleteCategory(ctx context.Context, key, moveTo string) (int, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var x int
	for _, k := range []string{key, moveTo} {
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM tag_categories WHERE cat_key = ?`, k).Scan(&x); errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		} else if err != nil {
			return 0, err
		}
	}
	res, err := tx.ExecContext(ctx, `UPDATE tags SET category = ? WHERE category = ?`, moveTo, key)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if _, err := tx.ExecContext(ctx, `DELETE FROM tag_categories WHERE cat_key = ?`, key); err != nil {
		return 0, err
	}
	return int(n), tx.Commit()
}

// ---------- Private Kontakte ----------

func (m *mysqlStore) ListPrivate(ctx context.Context, owner string) ([]PrivateContact, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id, last_name, first_name, phone, mobile, room, amt, email, note FROM private_contacts WHERE owner = ? ORDER BY last_name, first_name`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PrivateContact{}
	for rows.Next() {
		var p PrivateContact
		if err := rows.Scan(&p.ID, &p.LastName, &p.FirstName, &p.Phone, &p.Mobile, &p.Room, &p.Amt, &p.Email, &p.Note); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	shares, err := m.loadShares(ctx, `SELECT ps.contact_id, ps.kind, ps.value FROM private_shares ps JOIN private_contacts pc ON pc.id = ps.contact_id WHERE pc.owner = ?`, owner)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Shares = shares[out[i].ID]
	}
	return out, nil
}

// loadShares: Freigaben je Kontakt-ID
func (m *mysqlStore) loadShares(ctx context.Context, q string, args ...any) (map[int64]*Shares, error) {
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]*Shares{}
	for rows.Next() {
		var id int64
		var kind, val string
		if err := rows.Scan(&id, &kind, &val); err != nil {
			return nil, err
		}
		sh := out[id]
		if sh == nil {
			sh = &Shares{Employees: []int64{}, Amts: []string{}}
			out[id] = sh
		}
		if kind == "emp" {
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				sh.Employees = append(sh.Employees, n)
			}
		} else if kind == "amt" {
			sh.Amts = append(sh.Amts, val)
		}
	}
	return out, rows.Err()
}

func writeShares(ctx context.Context, tx *sql.Tx, id int64, sh *Shares) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM private_shares WHERE contact_id = ?`, id); err != nil {
		return err
	}
	if sh == nil {
		return nil
	}
	for _, e := range sh.Employees {
		if _, err := tx.ExecContext(ctx, `INSERT INTO private_shares (contact_id, kind, value) VALUES (?, 'emp', ?)`, id, strconv.FormatInt(e, 10)); err != nil && !isDuplicate(err) {
			return err
		}
	}
	for _, a := range sh.Amts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO private_shares (contact_id, kind, value) VALUES (?, 'amt', ?)`, id, a); err != nil && !isDuplicate(err) {
			return err
		}
	}
	return nil
}

func (m *mysqlStore) ListSharedWith(ctx context.Context, empID int64, amt, except string) ([]PrivateRec, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT DISTINCT pc.id, pc.owner, pc.last_name, pc.first_name, pc.phone, pc.mobile, pc.room, pc.amt, pc.email, pc.note, pc.created_at, pc.updated_at
		FROM private_contacts pc JOIN private_shares ps ON ps.contact_id = pc.id
		WHERE pc.owner <> ? AND ((ps.kind = 'emp' AND ps.value = ?) OR (ps.kind = 'amt' AND ? <> '' AND ps.value = ?))
		ORDER BY pc.id`, except, strconv.FormatInt(empID, 10), amt, amt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PrivateRec{}
	for rows.Next() {
		var p PrivateRec
		if err := rows.Scan(&p.ID, &p.Owner, &p.LastName, &p.FirstName, &p.Phone, &p.Mobile, &p.Room, &p.Amt, &p.Email, &p.Note, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (m *mysqlStore) CountPrivate(ctx context.Context, owner string) (int, error) {
	var n int
	err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM private_contacts WHERE owner = ?`, owner).Scan(&n)
	return n, err
}

func (m *mysqlStore) CreatePrivate(ctx context.Context, owner string, p PrivateContact) (int64, error) {
	now := time.Now().UTC()
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO private_contacts (owner, last_name, first_name, phone, mobile, room, amt, email, note, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		owner, p.LastName, p.FirstName, p.Phone, p.Mobile, p.Room, p.Amt, p.Email, p.Note, now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := writeShares(ctx, tx, id, p.Shares); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (m *mysqlStore) UpdatePrivate(ctx context.Context, owner string, p PrivateContact) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var x int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM private_contacts WHERE id = ? AND owner = ?`, p.ID, owner).Scan(&x); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE private_contacts SET last_name = ?, first_name = ?, phone = ?, mobile = ?, room = ?, amt = ?, email = ?, note = ?, updated_at = ? WHERE id = ? AND owner = ?`,
		p.LastName, p.FirstName, p.Phone, p.Mobile, p.Room, p.Amt, p.Email, p.Note, time.Now().UTC(), p.ID, owner); err != nil {
		return err
	}
	if err := writeShares(ctx, tx, p.ID, p.Shares); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *mysqlStore) DeletePrivate(ctx context.Context, owner string, id int64) error {
	res, err := m.db.ExecContext(ctx, `DELETE FROM private_contacts WHERE id = ? AND owner = ?`, id, owner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- Protokoll & Sync ----------

func (m *mysqlStore) AddAudit(ctx context.Context, e AuditEntry) error {
	_, err := m.db.ExecContext(ctx, `INSERT INTO audit_log (ts, actor, action, target, detail, ip) VALUES (?,?,?,?,?,?)`, e.TS, e.Actor, e.Action, e.Target, e.Detail, e.IP)
	return err
}

func (m *mysqlStore) ListAudit(ctx context.Context, q string, limit int) ([]AuditEntry, error) {
	sqlq := `SELECT ts, actor, action, target, COALESCE(detail,''), ip FROM audit_log`
	args := []any{}
	if q != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
		sqlq += ` WHERE actor LIKE ? OR action LIKE ? OR target LIKE ? OR detail LIKE ?`
		args = append(args, like, like, like, like)
	}
	sqlq += ` ORDER BY id DESC`
	if limit > 0 {
		sqlq += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := m.db.QueryContext(ctx, sqlq, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.TS, &e.Actor, &e.Action, &e.Target, &e.Detail, &e.IP); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (m *mysqlStore) StartSyncRun(ctx context.Context, trigger string, started time.Time) (int64, error) {
	r, err := m.db.ExecContext(ctx, `INSERT INTO sync_runs (started_at, trigger_by, status) VALUES (?,?, 'running')`, started, trigger)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func (m *mysqlStore) FinishSyncRun(ctx context.Context, r *SyncResult) error {
	_, err := m.db.ExecContext(ctx, `UPDATE sync_runs SET finished_at = ?, status = ?, seen = ?, created = ?, updated = ?, deactivated = ?, reactivated = ?, message = ? WHERE id = ?`,
		r.FinishedAt, r.Status, r.Seen, r.Created, r.Updated, r.Deactivated, r.Reactivated, r.Message, r.ID)
	return err
}

func (m *mysqlStore) ListSyncRuns(ctx context.Context, limit int) ([]SyncResult, error) {
	q := `SELECT id, started_at, finished_at, trigger_by, status, seen, created, updated, deactivated, reactivated, COALESCE(message,'') FROM sync_runs ORDER BY id DESC`
	args := []any{}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SyncResult{}
	for rows.Next() {
		var s SyncResult
		var fin sql.NullTime
		if err := rows.Scan(&s.ID, &s.StartedAt, &fin, &s.TriggerBy, &s.Status, &s.Seen, &s.Created, &s.Updated, &s.Deactivated, &s.Reactivated, &s.Message); err != nil {
			return nil, err
		}
		if fin.Valid {
			t := fin.Time
			s.FinishedAt = &t
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (m *mysqlStore) ApplySyncPlan(ctx context.Context, p *SyncPlan) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range p.Creates {
		if _, err := tx.ExecContext(ctx, `INSERT INTO employees (username, last_name, first_name, phone, mobile, room, amt, email, source, ad_guid, ad_dn, active, hidden, created_at, updated_at, last_synced_at) VALUES (?,?,?,?,?,?,?,?,'ad',?,?,1,0,?,?,?)`,
			nullStr(e.Username), e.LastName, e.FirstName, e.Phone, e.Mobile, e.Room, e.Amt, e.Email, nullStr(e.ADGUID), nullStr(e.ADDN), p.Now, p.Now, p.Now); err != nil {
			return err
		}
	}
	for _, u := range p.Updates {
		if u.SetUsername != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE employees SET username = ? WHERE id = ? AND username IS NULL`, u.SetUsername, u.ID); err != nil && !isDuplicate(err) {
				return err
			}
		}
		f := u.Fields
		if _, err := tx.ExecContext(ctx, `UPDATE employees SET last_name = ?, first_name = ?, phone = ?, mobile = ?, room = ?, amt = ?, email = ?, source = 'ad', ad_guid = COALESCE(?, ad_guid), ad_dn = ?, active = 1, last_synced_at = ?, updated_at = CASE WHEN ? THEN ? ELSE updated_at END WHERE id = ?`,
			f.LastName, f.FirstName, f.Phone, f.Mobile, f.Room, f.Amt, f.Email, nullStr(u.GUID), u.DN, p.Now, u.Changed, p.Now, u.ID); err != nil {
			return err
		}
	}
	for _, id := range p.Deactivate {
		if _, err := tx.ExecContext(ctx, `UPDATE employees SET active = 0, updated_at = ? WHERE id = ?`, p.Now, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- Gesamtabzug ----------

func (m *mysqlStore) Export(ctx context.Context) (*Snapshot, error) {
	s := &Snapshot{Format: snapshotFormat, Version: 1, ExportedAt: time.Now().UTC(), Source: "mysql"}
	var err error
	if s.Employees, err = m.ListEmployees(ctx, true); err != nil {
		return nil, err
	}
	rows, err := m.db.QueryContext(ctx, `SELECT id, name, category, created_by, created_at FROM tags ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t TagRec
		if err := rows.Scan(&t.ID, &t.Name, &t.Category, &t.CreatedBy, &t.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		s.Tags = append(s.Tags, t)
	}
	rows.Close()
	if s.Categories, err = m.ListCategories(ctx); err != nil {
		return nil, err
	}
	for i := range s.Categories {
		s.Categories[i].Count = 0
	}
	allShares, err := m.loadShares(ctx, `SELECT contact_id, kind, value FROM private_shares`)
	if err != nil {
		return nil, err
	}
	rows, err = m.db.QueryContext(ctx, `SELECT id, owner, last_name, first_name, phone, mobile, room, amt, email, note, created_at, updated_at FROM private_contacts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p PrivateRec
		if err := rows.Scan(&p.ID, &p.Owner, &p.LastName, &p.FirstName, &p.Phone, &p.Mobile, &p.Room, &p.Amt, &p.Email, &p.Note, &p.CreatedAt, &p.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		p.Shares = allShares[p.ID]
		s.Private = append(s.Private, p)
	}
	rows.Close()
	rows, err = m.db.QueryContext(ctx, `SELECT username, display_name, pw_hash, is_admin, disabled, created_at, last_login FROM local_users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var u LocalUser
		var ll sql.NullTime
		if err := rows.Scan(&u.Username, &u.DisplayName, &u.PwHash, &u.IsAdmin, &u.Disabled, &u.CreatedAt, &ll); err != nil {
			rows.Close()
			return nil, err
		}
		if ll.Valid {
			t := ll.Time
			u.LastLogin = &t
		}
		s.LocalUsers = append(s.LocalUsers, u)
	}
	rows.Close()
	if s.Audit, err = m.ListAudit(ctx, "", jsonAuditMax); err != nil {
		return nil, err
	}
	if s.SyncRuns, err = m.ListSyncRuns(ctx, jsonSyncRunsMax); err != nil {
		return nil, err
	}
	if s.OrgUnits, err = m.ListOrgUnits(ctx); err != nil {
		return nil, err
	}
	if s.Reports, err = m.ListReports(ctx, "", 0); err != nil {
		return nil, err
	}
	if s.Channels, err = m.ListChannels(ctx); err != nil {
		return nil, err
	}
	rows, err = m.db.QueryContext(ctx, `SELECT k, v FROM settings`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			rows.Close()
			return nil, err
		}
		if s.Settings == nil {
			s.Settings = map[string]string{}
		}
		s.Settings[k] = v
	}
	rows.Close()
	s.normalize()
	return s, nil
}

func (m *mysqlStore) Import(ctx context.Context, s *Snapshot) error {
	if err := s.validate(); err != nil {
		return err
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range []string{"employee_tags", "employees", "tags", "private_shares", "private_contacts", "tag_categories", "local_users", "audit_log", "sync_runs", "sessions", "org_units", "settings", "reports", "contact_channels"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+t); err != nil {
			return err
		}
	}
	for _, c := range s.Channels {
		if _, err := tx.ExecContext(ctx, `INSERT INTO contact_channels (id, name, ticket_url, ticket_label, email, phone, note, hide_direct) VALUES (?,?,?,?,?,?,?,?)`,
			c.ID, c.Name, c.TicketURL, c.TicketLabel, c.Email, c.Phone, c.Note, c.HideDirect); err != nil {
			return err
		}
	}
	for _, u := range s.OrgUnits {
		if _, err := tx.ExecContext(ctx, `INSERT INTO org_units (id, parent_id, name, short_name, sort, leaders, deputies, contact) VALUES (?,?,?,?,?,?,?,?)`,
			u.ID, nullID(u.Parent), u.Name, u.Short, u.Sort, jsonCol(u.Leaders), jsonCol(u.Deputies), contactCol(u.Contact)); err != nil {
			return err
		}
	}
	for k, v := range s.Settings {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (k, v) VALUES (?,?)`, k, v); err != nil {
			return err
		}
	}
	for _, r := range s.Reports {
		if _, err := tx.ExecContext(ctx, `INSERT INTO reports (id, employee_id, target, field, suggestion, message, reporter, reporter_name, created_at, status, resolved_by, resolved_at, note) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			r.ID, nullID(r.EmployeeID), r.Target, r.Field, r.Suggestion, r.Message, r.Reporter, r.ReporterName, r.CreatedAt, r.Status, r.ResolvedBy, r.ResolvedAt, r.Note); err != nil {
			return err
		}
	}
	for _, c := range s.Categories {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tag_categories (cat_key, name, color, sort) VALUES (?,?,?,?)`, c.Key, c.Name, c.Color, c.Sort); err != nil {
			return err
		}
	}
	for _, t := range s.Tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tags (id, name, category, created_by, created_at) VALUES (?,?,?,?,?)`, t.ID, t.Name, t.Category, t.CreatedBy, t.CreatedAt); err != nil {
			return err
		}
	}
	for _, e := range s.Employees {
		if _, err := tx.ExecContext(ctx, `INSERT INTO employees (id, username, last_name, first_name, phone, mobile, room, amt, email, source, ad_guid, ad_dn, active, hidden, avatar, created_at, updated_at, last_synced_at, org_unit_id, deputies, absences, channel_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			e.ID, nullStr(e.Username), e.LastName, e.FirstName, e.Phone, e.Mobile, e.Room, e.Amt, e.Email, e.Source, nullStr(e.ADGUID), nullStr(e.ADDN), e.Active, e.Hidden, nullRaw(e.Avatar), e.CreatedAt, e.UpdatedAt, e.LastSyncedAt, nullID(e.OrgUnit), jsonCol(e.Deputies), jsonCol(e.Absences), nullID(e.Channel)); err != nil {
			return err
		}
		for _, t := range e.Tags {
			if _, err := tx.ExecContext(ctx, `INSERT INTO employee_tags (employee_id, tag_id, created_at) VALUES (?,?,?)`, e.ID, t, e.UpdatedAt); err != nil {
				return err
			}
		}
	}
	for _, p := range s.Private {
		if _, err := tx.ExecContext(ctx, `INSERT INTO private_contacts (id, owner, last_name, first_name, phone, mobile, room, amt, email, note, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.ID, p.Owner, p.LastName, p.FirstName, p.Phone, p.Mobile, p.Room, p.Amt, p.Email, p.Note, p.CreatedAt, p.UpdatedAt); err != nil {
			return err
		}
		if err := writeShares(ctx, tx, p.ID, p.Shares); err != nil {
			return err
		}
	}
	for _, u := range s.LocalUsers {
		if _, err := tx.ExecContext(ctx, `INSERT INTO local_users (username, display_name, pw_hash, is_admin, disabled, created_at, last_login) VALUES (?,?,?,?,?,?,?)`,
			u.Username, u.DisplayName, u.PwHash, u.IsAdmin, u.Disabled, u.CreatedAt, u.LastLogin); err != nil {
			return err
		}
	}
	for i := len(s.Audit) - 1; i >= 0; i-- { // älteste zuerst
		a := s.Audit[i]
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_log (ts, actor, action, target, detail, ip) VALUES (?,?,?,?,?,?)`, a.TS, a.Actor, a.Action, a.Target, a.Detail, a.IP); err != nil {
			return err
		}
	}
	for i := len(s.SyncRuns) - 1; i >= 0; i-- {
		r := s.SyncRuns[i]
		if _, err := tx.ExecContext(ctx, `INSERT INTO sync_runs (started_at, finished_at, trigger_by, status, seen, created, updated, deactivated, reactivated, message) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			r.StartedAt, r.FinishedAt, r.TriggerBy, r.Status, r.Seen, r.Created, r.Updated, r.Deactivated, r.Reactivated, r.Message); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nullID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

// removeRefsTx entfernt einen gelöschten Eintrag aus Vertretungen, Abwesenheits-Vertretungen und Einheiten.
func removeRefsTx(ctx context.Context, tx *sql.Tx, id int64) error {
	type row struct {
		id         int64
		deps, abss string
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, COALESCE(deputies,''), COALESCE(absences,'') FROM employees WHERE deputies IS NOT NULL OR absences IS NOT NULL`)
	if err != nil {
		return err
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.deps, &r.abss); err != nil {
			rows.Close()
			return err
		}
		list = append(list, r)
	}
	rows.Close()
	for _, r := range list {
		deps, hit := removeID(parseCol[int64](r.deps), id)
		abs := parseCol[Absence](r.abss)
		for i := range abs {
			var h bool
			abs[i].Deputies, h = removeID(abs[i].Deputies, id)
			hit = hit || h
		}
		if hit {
			if _, err := tx.ExecContext(ctx, `UPDATE employees SET deputies = ?, absences = ? WHERE id = ?`, jsonCol(deps), jsonCol(abs), r.id); err != nil {
				return err
			}
		}
	}
	urows, err := tx.QueryContext(ctx, `SELECT id, COALESCE(leaders,''), COALESCE(deputies,'') FROM org_units WHERE leaders IS NOT NULL OR deputies IS NOT NULL`)
	if err != nil {
		return err
	}
	list = list[:0]
	for urows.Next() {
		var r row
		if err := urows.Scan(&r.id, &r.deps, &r.abss); err != nil {
			urows.Close()
			return err
		}
		list = append(list, r)
	}
	urows.Close()
	for _, r := range list {
		l, h1 := removeID(parseCol[int64](r.deps), id)
		d, h2 := removeID(parseCol[int64](r.abss), id)
		if h1 || h2 {
			if _, err := tx.ExecContext(ctx, `UPDATE org_units SET leaders = ?, deputies = ? WHERE id = ?`, jsonCol(l), jsonCol(d), r.id); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------- Vertretungen, Abwesenheiten, Organisation ----------

func (m *mysqlStore) SetDeputies(ctx context.Context, id int64, deps []int64) error {
	res, err := m.db.ExecContext(ctx, `UPDATE employees SET deputies = ? WHERE id = ?`, jsonCol(deps), id)
	if err != nil {
		return err
	}
	return m.mustExist(ctx, res, `SELECT 1 FROM employees WHERE id = ?`, id)
}

func (m *mysqlStore) SetAbsences(ctx context.Context, id int64, abs []Absence) error {
	res, err := m.db.ExecContext(ctx, `UPDATE employees SET absences = ? WHERE id = ?`, jsonCol(abs), id)
	if err != nil {
		return err
	}
	return m.mustExist(ctx, res, `SELECT 1 FROM employees WHERE id = ?`, id)
}

func (m *mysqlStore) SetOrgUnit(ctx context.Context, ids []int64, unit int64) (int, error) {
	n := 0
	for _, id := range ids {
		res, err := m.db.ExecContext(ctx, `UPDATE employees SET org_unit_id = ? WHERE id = ?`, nullID(unit), id)
		if err != nil {
			return n, err
		}
		k, _ := res.RowsAffected()
		n += int(k)
	}
	return n, nil
}

func (m *mysqlStore) ListOrgUnits(ctx context.Context) ([]OrgUnit, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id, COALESCE(parent_id,0), name, short_name, sort, COALESCE(leaders,''), COALESCE(deputies,''), COALESCE(contact,'') FROM org_units ORDER BY sort, name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrgUnit{}
	for rows.Next() {
		var u OrgUnit
		var l, d, c string
		if err := rows.Scan(&u.ID, &u.Parent, &u.Name, &u.Short, &u.Sort, &l, &d, &c); err != nil {
			return nil, err
		}
		if c != "" {
			var cp ContactPref
			if json.Unmarshal([]byte(c), &cp) == nil {
				u.Contact = &cp
			}
		}
		u.Leaders, u.Deputies = parseCol[int64](l), parseCol[int64](d)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (m *mysqlStore) CreateOrgUnit(ctx context.Context, u OrgUnit) (int64, error) {
	res, err := m.db.ExecContext(ctx, `INSERT INTO org_units (parent_id, name, short_name, sort, leaders, deputies, contact, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		nullID(u.Parent), u.Name, u.Short, u.Sort, jsonCol(u.Leaders), jsonCol(u.Deputies), contactCol(u.Contact), time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *mysqlStore) UpdateOrgUnit(ctx context.Context, u OrgUnit) error {
	res, err := m.db.ExecContext(ctx, `UPDATE org_units SET parent_id = ?, name = ?, short_name = ?, sort = ?, leaders = ?, deputies = ?, contact = ? WHERE id = ?`,
		nullID(u.Parent), u.Name, u.Short, u.Sort, jsonCol(u.Leaders), jsonCol(u.Deputies), contactCol(u.Contact), u.ID)
	if err != nil {
		return err
	}
	return m.mustExist(ctx, res, `SELECT 1 FROM org_units WHERE id = ?`, u.ID)
}

func (m *mysqlStore) DeleteOrgUnit(ctx context.Context, id int64) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var parent int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(parent_id,0) FROM org_units WHERE id = ?`, id).Scan(&parent); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE org_units SET parent_id = ? WHERE parent_id = ?`, nullID(parent), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE employees SET org_unit_id = ? WHERE org_unit_id = ?`, nullID(parent), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM org_units WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *mysqlStore) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := m.db.QueryRowContext(ctx, `SELECT v FROM settings WHERE k = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (m *mysqlStore) SetSetting(ctx context.Context, key, value string) error {
	_, err := m.db.ExecContext(ctx, `INSERT INTO settings (k, v) VALUES (?, ?) ON DUPLICATE KEY UPDATE v = VALUES(v)`, key, value)
	return err
}

// ---------- Fehlermeldungen ----------

func (m *mysqlStore) CreateReport(ctx context.Context, r Report) (int64, error) {
	res, err := m.db.ExecContext(ctx, `INSERT INTO reports (employee_id, target, field, suggestion, message, reporter, reporter_name, created_at, status) VALUES (?,?,?,?,?,?,?,?,'open')`,
		nullID(r.EmployeeID), r.Target, r.Field, r.Suggestion, r.Message, r.Reporter, r.ReporterName, r.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *mysqlStore) ListReports(ctx context.Context, status string, limit int) ([]Report, error) {
	q := `SELECT id, COALESCE(employee_id,0), target, field, suggestion, message, reporter, reporter_name, created_at, status, resolved_by, resolved_at, note FROM reports`
	args := []any{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC, id DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Report{}
	for rows.Next() {
		var r Report
		var ra sql.NullTime
		if err := rows.Scan(&r.ID, &r.EmployeeID, &r.Target, &r.Field, &r.Suggestion, &r.Message, &r.Reporter, &r.ReporterName, &r.CreatedAt, &r.Status, &r.ResolvedBy, &ra, &r.Note); err != nil {
			return nil, err
		}
		if ra.Valid {
			t := ra.Time
			r.ResolvedAt = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (m *mysqlStore) UpdateReport(ctx context.Context, id int64, status, by, note string) error {
	var at any
	if status != "open" {
		at = time.Now().UTC()
	} else {
		by = ""
	}
	res, err := m.db.ExecContext(ctx, `UPDATE reports SET status = ?, resolved_by = ?, resolved_at = ?, note = ? WHERE id = ?`, status, by, at, note, id)
	if err != nil {
		return err
	}
	return m.mustExist(ctx, res, `SELECT 1 FROM reports WHERE id = ?`, id)
}

func (m *mysqlStore) CountReports(ctx context.Context, status, reporter string) (int, error) {
	q := `SELECT COUNT(*) FROM reports WHERE status = ?`
	args := []any{status}
	if reporter != "" {
		q += ` AND reporter = ?`
		args = append(args, reporter)
	}
	var n int
	err := m.db.QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

func contactCol(c *ContactPref) any {
	if c == nil {
		return nil
	}
	b, _ := json.Marshal(c)
	return string(b)
}

// ---------- Kontaktwege ----------

func (m *mysqlStore) ListChannels(ctx context.Context) ([]ContactChannel, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id, name, ticket_url, ticket_label, email, phone, note, hide_direct FROM contact_channels ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContactChannel{}
	for rows.Next() {
		var c ContactChannel
		if err := rows.Scan(&c.ID, &c.Name, &c.TicketURL, &c.TicketLabel, &c.Email, &c.Phone, &c.Note, &c.HideDirect); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (m *mysqlStore) CreateChannel(ctx context.Context, c ContactChannel) (int64, error) {
	res, err := m.db.ExecContext(ctx, `INSERT INTO contact_channels (name, ticket_url, ticket_label, email, phone, note, hide_direct, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		c.Name, c.TicketURL, c.TicketLabel, c.Email, c.Phone, c.Note, c.HideDirect, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *mysqlStore) UpdateChannel(ctx context.Context, c ContactChannel) error {
	res, err := m.db.ExecContext(ctx, `UPDATE contact_channels SET name = ?, ticket_url = ?, ticket_label = ?, email = ?, phone = ?, note = ?, hide_direct = ? WHERE id = ?`,
		c.Name, c.TicketURL, c.TicketLabel, c.Email, c.Phone, c.Note, c.HideDirect, c.ID)
	if err != nil {
		return err
	}
	return m.mustExist(ctx, res, `SELECT 1 FROM contact_channels WHERE id = ?`, c.ID)
}

func (m *mysqlStore) DeleteChannel(ctx context.Context, id int64) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE employees SET channel_id = NULL WHERE channel_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM contact_channels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (m *mysqlStore) SetChannel(ctx context.Context, ids []int64, channel int64) (int, error) {
	n := 0
	for _, id := range ids {
		res, err := m.db.ExecContext(ctx, `UPDATE employees SET channel_id = ? WHERE id = ?`, nullID(channel), id)
		if err != nil {
			return n, err
		}
		k, _ := res.RowsAffected()
		n += int(k)
	}
	return n, nil
}
