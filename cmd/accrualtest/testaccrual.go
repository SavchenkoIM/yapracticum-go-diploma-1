package main

import "yapracticum-go-diploma-1/internal/accrualstab"

func main() {
	s := accrualstab.NewAccrualStab("localhost:8090")
	err := s.Serve()
	if err != nil {
		return
	}
}
