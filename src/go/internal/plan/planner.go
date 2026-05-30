package plan

import (
	"context"
	"fmt"
	"time"
)

type Plan struct {
	Title       string
	Steps       []Step
	CreatedAt   time.Time
	FileContent string
}

type Step struct {
	Description string
	Status      string
	Files       []string
}

type Planner struct {
	currentPlan *Plan
}

func NewPlanner() *Planner {
	return &Planner{}
}

func (p *Planner) EnterPlanMode() {
	fmt.Println(">>> Entering plan mode (read-only)...")
}

func (p *Planner) ExitPlanMode(planFile string) error {
	fmt.Printf(">>> Plan submitted: %s\n", planFile)
	return nil
}

func (p *Planner) TrackProgress(ctx context.Context, plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("no plan to track")
	}
	fmt.Printf("Tracking plan: %s (%d steps)\n", plan.Title, len(plan.Steps))
	for i, step := range plan.Steps {
		plan.Steps[i].Status = "in_progress"
		fmt.Printf("  [%d/%d] %s...\n", i+1, len(plan.Steps), step.Description)
		plan.Steps[i].Status = "completed"
	}
	fmt.Println("Plan complete.")
	return nil
}
