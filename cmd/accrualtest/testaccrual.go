package main

import "yapracticum-go-diploma-1/internal/accrualstab"

func main() {
	s := accrualstab.NewAccrualStab("localhost:8090", "localhost:6379")
	err := s.Serve()
	if err != nil {
		return
	}
}
