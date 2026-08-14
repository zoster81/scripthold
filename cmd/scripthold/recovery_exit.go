package main

import "github.com/zoster81/scripthold/internal/backupstore"

func backupRecoveryPlanExitCode(plan backupstore.RecoveryPlan, err error) int {
	if err != nil {
		return 1
	}
	if !plan.Applicable || !plan.CoverageComplete {
		return 2
	}
	return 0
}

func backupRecoveryApplyExitCode(err error) int {
	if err != nil {
		return 1
	}
	return 0
}
