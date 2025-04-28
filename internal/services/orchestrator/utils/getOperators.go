package orchestratorutils

import (
	"time"

	"github.com/neandrson/go_calc_final/internal/config"
	"github.com/neandrson/go_calc_final/internal/models"
)

// GetOperators возвращает список операций
func GetOperators(timeouts config.CalculationTimeoutsConfig) []*models.Operator {
	operatorsMap := map[string]time.Duration{
		"+": timeouts.TimeCalculatePlus / time.Second,
		"-": timeouts.TimeCalculateMinus / time.Second,
		"*": timeouts.TimeCalculateMult / time.Second,
		"/": timeouts.TimeCalculateDivide / time.Second,
	}
	var operators []*models.Operator
	for key, value := range operatorsMap {
		operators = append(operators, &models.Operator{Op: key, Timeout: value})
	}
	return operators
}
