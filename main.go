package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

const defaultEmail = "betplacer@gmail.com"
const alreadyExistsPostgresCode = "23505"

type BetController struct {
	db *sql.DB
}

type BetPayload struct {
	State         string
	Amount        string
	BetID string
}

type User struct {
	ID      int
	Balance int
	Email   string
}

type Bet struct {
	ExternalID string
	UserID     int
	Type       string
	Amount     int
	SourceType int
	Processed  bool
	CreatedAt  time.Time
}

func main() {
	db, err := sql.Open("postgres", "postgres://postgres:postgres@database/bets?sslmode=disable")
	if err != nil {
		log.Fatalf("failed to connect to the database: %s", err)
	}

	r := mux.NewRouter()
	tc := NewBetController(db)
	r.HandleFunc("/bet", tc.Handle).Methods(http.MethodPost)
	log.Println("Server accepting connections...")

	go func() {
		ticker := time.NewTicker(time.Second * 10)
		for range ticker.C {
			err := PostProcess(db)
			if err != nil {
				log.Println(err)
			}
		}
	}()

	err = http.ListenAndServe("0.0.0.0:8083", r)
	if err != nil {
		log.Fatal(err)
	}
}

func NewBetController(db *sql.DB) *BetController {
	return &BetController{db: db}
}

func (tc *BetController) Handle(rw http.ResponseWriter, r *http.Request) {
	var betPayload BetPayload
	err := json.NewDecoder(r.Body).Decode(&betPayload)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	tx, err := tc.db.BeginTx(r.Context(), &sql.TxOptions{})
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var sourceID int
	row := tx.QueryRow("SELECT id FROM sources WHERE value = $1", r.Header.Get("Source-Type"))
	err = row.Scan(&sourceID)
	if err != nil {
		if err == sql.ErrNoRows {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	var user User
	row = tx.QueryRow("SELECT  * FROM users  WHERE email=$1 FOR UPDATE", defaultEmail)
	err = row.Scan(&user.ID, &user.Balance, &user.Email)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	amountF, err := strconv.ParseFloat(betPayload.Amount, 64)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	amount := int64(amountF * 1000)

	_, err = tx.Exec(`INSERT INTO bets (external_id, user_id, type, amount, source_type) 
		VALUES ($1, $2, $3, $4, $5)`,
		betPayload.BetID, user.ID, betPayload.State, amount, sourceID)
	if err != nil {
		pqErr, ok := err.(*pq.Error)
		// if bet is already in place we quietly proceed with 200
		if ok && pqErr.Code == alreadyExistsPostgresCode {
			rw.WriteHeader(http.StatusOK)
			return
		}
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	if betPayload.State == "win" {
		user.Balance += int(amount)
	} else if betPayload.State == "lost" {
		user.Balance -= int(amount)
	} else {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	if user.Balance < 0 {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	_, err = tx.Exec(`UPDATE users SET balance = $1 WHERE id = $2`, user.Balance, user.ID)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	err = tx.Commit()
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

}

func PostProcess(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	var user User
	row := tx.QueryRow("SELECT  * FROM users  WHERE email=$1 FOR UPDATE", defaultEmail)
	err = row.Scan(&user.ID, &user.Balance, &user.Email)
	if err != nil {
		return fmt.Errorf("query users: %w", err)
	}

	defer tx.Rollback()
	rows, err := tx.Query("SELECT * FROM bets WHERE processed=false ORDER BY created_at DESC LIMIT 10")
	if err != nil {
		return fmt.Errorf("query bets: %w", err)
	}
	var balanceDelta int
	var bets []Bet
	for rows.Next() {
		var bet Bet
		err := rows.Scan(&bet.ExternalID, &bet.UserID, &bet.Type, &bet.Amount, &bet.SourceType, &bet.Processed, &bet.CreatedAt)
		if err != nil {
			return err
		}
		if bet.Type == "win" {
			balanceDelta -= bet.Amount
		} else if bet.Type == "lost" {
			balanceDelta += bet.Amount
		}
		bets = append(bets, bet)
	}

	for _, t := range bets {
		_, err = tx.Exec("UPDATE bets SET processed = $1 WHERE external_id = $2", true, t.ExternalID)
		if err != nil {
			return fmt.Errorf("update bets processed: %w", err)
		}
	}
	user.Balance += balanceDelta
	_, err = tx.Exec(`UPDATE users SET balance = $1 WHERE id = $2`, user.Balance, user.ID)
	if err != nil {
		return fmt.Errorf("update user balance: %w", err)
	}
	return tx.Commit()
}
