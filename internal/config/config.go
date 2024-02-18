package config

import (
	"flag"
	"os"
)

type Config struct {
	ConnString     string
	Endpoint       string
	AccrualAddress string
	UseLuna        bool
}

func New() Config {
	var res Config

	pConnString := flag.String("d", "", "Database connection string")
	pEndpoint := flag.String("a", ":8080", "Server endpoint")
	pAccrualAddress := flag.String("r", "localhost:8090", "Accrual system address")
	pUseLuna := flag.Bool("useLuna", true, "Is Luna required")
	flag.Parse()

	if val, ok := os.LookupEnv("DATABASE_URI"); ok {
		pConnString = &val
	}
	if val, ok := os.LookupEnv("RUN_ADDRESS"); ok {
		pEndpoint = &val
	}
	if val, ok := os.LookupEnv("ACCRUAL_SYSTEM_ADDRESS"); ok {
		pAccrualAddress = &val
	}

	res.ConnString = *pConnString
	res.Endpoint = *pEndpoint
	res.AccrualAddress = *pAccrualAddress
	res.UseLuna = *pUseLuna

	return res
}
