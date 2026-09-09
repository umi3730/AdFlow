package main

import "fmt"

func validatePoolFlags(open, idle int, backpressure bool) error {
	if open < 0 || open > 100 || idle < 0 || (open > 0 && (!backpressure || idle > open)) || (open == 0 && idle != 10) {
		return fmt.Errorf("pool comparison requires -backpressure, max-open=1..100, and 0 <= max-idle <= max-open")
	}
	return nil
}
