package main

import (
	"log"

	"github.com/kr/pretty"

	"github.com/go-pg/pg"
)

type postgresHelper struct {
	db *pg.DB
}

func NewPostgresHelper(myConf *conf) *postgresHelper {
	db := pg.Connect(&pg.Options{
		Addr:     myConf.PgHost + ":5432",
		User:     "postgres",
		Password: myConf.PgPass,
		Database: "postgres",
	})

	// var horselegs struct {
	// 	Legs int
	// }

	// res, err := db.QueryOne(&horselegs, `SELECT * FROM horse`)
	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Println(res.RowsAffected())
	// fmt.Println(horselegs)

	// defer db.Close()

	return &postgresHelper{db}
}

func (pgh *postgresHelper) closeDb() {
	log.Printf("Closing db connection.")
	pgh.db.Close()
}

func (pgh *postgresHelper) savePaidInvoiceDetail(record map[interface{}]interface{}) {
	_, err := pgh.db.Exec(`INSERT INTO paidinvoice (earmark, attribution, sats, rhash) VALUES (?, ?, ?, ?)`, record["invoice-earmark"].(string), record["invoice-attribution"].(string), record["invoice-sats"].(int64), record["invoice-rhash"].(string))
	if err != nil {
		panic(err)
	}
	// CREATE TABLE paidInvoice (
	// id serial PRIMARY KEY,
	// earmark VARCHAR (128) NULL,
	// attribution VARCHAR (128) NULL,
	// sats int NOT NULL,
	// rhash CHAR(64) NOT NULL,
	// created_on TIMESTAMP NOT NULL DEFAULT NOW()
	// );

	log.Printf("should have saved this info into db: %#v", pretty.Formatter(record))
}
