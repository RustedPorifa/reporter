package godb

import (
	"database/sql"
	"fmt"
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

	// Создаем таблицу если база только что создана
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS admins (
		id INTEGER PRIMARY KEY,
		role TEXT NOT NULL
	);`

	if _, err := db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create table: %v", err)
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

// PrintAllAdmins выводит всех администраторов
func PrintAllAdmins() error {
	rows, err := db.Query("SELECT id, role FROM admins")
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Println("Administrators:")
	for rows.Next() {
		var id int64
		var role string
		if err := rows.Scan(&id, &role); err != nil {
			return err
		}
		fmt.Printf("- %d (%s)\n", id, role)
	}
	return rows.Err()
}
