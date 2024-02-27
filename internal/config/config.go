package config

import (
	"flag"
	"os"
)

type Config struct {
	ConnString     string
	Endpoint       string
	AccrualAddress string
	UseLuhn        bool
}

func New() Config {
	var res Config

	pConnString := flag.String("d", "", "Database connection string")
	pEndpoint := flag.String("a", ":8080", "Server endpoint")
	pAccrualAddress := flag.String("r", "localhost:8090", "Accrual system address")
	pUseLuhn := flag.Bool("useLuhn", true, "Is Luhn required")
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
	res.UseLuhn = *pUseLuhn

	return res
}
