package godb

import (
	"database/sql"
	"fmt"
	"math/rand"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

// InitDB инициализирует базу данных и создает файл admins.db если он не существует
func InitDB() error {
	// Проверяем существование файла
	_, err := os.Stat("admins.db")
	dbNotExists := os.IsNotExist(err)

	// Открываем/создаем базу данных
	db, err = sql.Open("sqlite3", "admins.db")
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	// Проверяем соединение
	if err := db.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %v", err)
	}

	// Создаем таблицу администраторов
	createAdminsTable := `
	CREATE TABLE IF NOT EXISTS admins (
		id INTEGER PRIMARY KEY,
		role TEXT NOT NULL
	);`
	if _, err := db.Exec(createAdminsTable); err != nil {
		return fmt.Errorf("failed to create admins table: %v", err)
	}

	// Создаем таблицу прокси
	createProxiesTable := `
	CREATE TABLE IF NOT EXISTS proxies (
		name TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		value INTEGER NOT NULL
	);`
	if _, err := db.Exec(createProxiesTable); err != nil {
		return fmt.Errorf("failed to create proxies table: %v", err)
	}

	// Для отладки: сообщаем о создании новой БД
	if dbNotExists {
		fmt.Println("Created new database file: admins.db")
	}

	return nil
}

// CloseDB закрывает соединение с базой данных
func CloseDB() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

// AddOrUpdateAdmin добавляет или обновляет запись администратора
func AddOrUpdateAdmin(id int64, isAdmin bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	// Удаляем существующую запись
	if _, err := tx.Exec("DELETE FROM admins WHERE id = ?", id); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete failed: %v", err)
	}

	// Добавляем только если нужно назначить администратором
	if isAdmin {
		if _, err := tx.Exec(
			"INSERT INTO admins (id, role) VALUES (?, ?)",
			id,
			"admin",
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert failed: %v", err)
		}
	}

	return tx.Commit()
}

// IsAdmin проверяет наличие прав администратора
func IsAdmin(id int64) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM admins WHERE id = ? AND role = 'admin')"
	err := db.QueryRow(query, id).Scan(&exists)
	return exists, err
}

// Возвращает массив ID всех администраторов
func GetAllAdmins() ([]int64, error) {
	rows, err := db.Query("SELECT id FROM admins WHERE role = 'admin'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var admins []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		admins = append(admins, id)
	}

	// Проверяем ошибки итерации
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return admins, nil
}

func AddProxy(name, url string, value int) error {
	_, err := db.Exec(`
		INSERT INTO proxies (name, url, value) 
		VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			url = excluded.url,
			value = excluded.value
	`, name, url, value)
	return err
}

// GetProxyValue возвращает значение прокси по имени
func GetProxyValue(name string) (int, error) {
	var value int
	err := db.QueryRow("SELECT value FROM proxies WHERE name = ?", name).Scan(&value)
	return value, err
}

// GetRandomProxyBelow возвращает случайную прокси с значением <= maxValue
func GetRandomProxyBelow(maxValue int) (name, url string, err error) {
	// Сначала получаем количество подходящих прокси
	var count int
	err = db.QueryRow(`
		SELECT COUNT(*) 
		FROM proxies 
		WHERE value <= ?
	`, maxValue).Scan(&count)
	if err != nil {
		return "", "", err
	}
	if count == 0 {
		return "", "", sql.ErrNoRows
	}

	offset := rand.Intn(count)

	// Выбираем прокси по случайному смещению
	row := db.QueryRow(`
		SELECT name, url 
		FROM proxies 
		WHERE value <= ?
		LIMIT 1 OFFSET ?
	`, maxValue, offset)

	err = row.Scan(&name, &url)
	return name, url, err
}

// GetProxyCount возвращает количество прокси в базе данных в виде строки
func GetProxyCount() (string, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM proxies").Scan(&count)
	if err != nil {
		return "", fmt.Errorf("failed to get proxy count: %v", err)
	}
	return fmt.Sprintf("%d", count), nil
}
