package stefunny

import (
	"context"
	"errors"
	"fmt"
	"log"
)

func (app *App) Deploy(ctx context.Context, opt DeployOption) error {
	log.Println("[info] Starting deploy", opt.DryRunString())
	if err := app.deployStateMachine(ctx, opt); err != nil {
		return err
	}
	if err := app.deployScheduleRule(ctx, opt); err != nil {
		return err
	}
	log.Println("[info] finish deploy", opt.DryRunString())
	return nil
}

func (app *App) deployStateMachine(ctx context.Context, opt DeployOption) error {
	stateMachine, err := app.aws.DescribeStateMachine(ctx, app.cfg.StateMachine.Name)
	if err != nil {
		if err == ErrStateMachineDoesNotExist {
			return app.createStateMachine(ctx, opt)
		}
		return fmt.Errorf("failed to describe current state machine status: %w", err)
	}
	newStateMachine, err := app.LoadStateMachine(ctx)
	if err != nil {
		return err
	}
	newStateMachine.StateMachineArn = stateMachine.StateMachineArn
	if opt.DryRun {
		diffString := stateMachine.DiffString(newStateMachine)
		log.Printf("[notice] change state machine %s\n%s", opt.DryRunString(), diffString)
		return nil
	}
	output, err := app.aws.DeployStateMachine(ctx, newStateMachine)
	if err != nil {
		return err
	}
	log.Printf("[info] deploy state machine `%s`(at `%s`)\n", app.cfg.StateMachine.Name, *output.UpdateDate)
	return nil
}

func (app *App) deployScheduleRule(ctx context.Context, opt DeployOption) error {
	stateMachineArn, err := app.aws.GetStateMachineArn(ctx, app.cfg.StateMachine.Name)
	if err != nil {
		if err == ErrStateMachineDoesNotExist {
			return app.createScheduleRule(ctx, opt)
		}
		return fmt.Errorf("failed to get state machine arn: %w", err)
	}
	ruleName := getScheduleRuleName(app.cfg.StateMachine.Name)
	rule, err := app.aws.DescribeScheduleRule(ctx, ruleName)
	if err != nil {
		if err == ErrScheduleRuleDoesNotExist {
			if app.cfg.Schedule == nil {
				return nil
			} else {
				return app.createScheduleRule(ctx, opt)
			}
		}
		return err
	}
	var newRule *ScheduleRule
	if app.cfg.Schedule != nil {
		var err error
		newRule, err = app.LoadScheduleRule(ctx)
		if err != nil {
			return err
		}
		newRule.SetStateMachineArn(stateMachineArn)
	}
	if opt.DryRun {
		diffString := rule.DiffString(newRule)
		log.Printf("[notice] change schedule rule %s\n%s", opt.DryRunString(), diffString)
		return nil
	}
	if newRule == nil {
		err := app.aws.DeleteScheduleRule(ctx, rule)
		if err != nil {
			return err
		}
		log.Printf("[info] delete schdule rule")
		return nil
	}
	output, err := app.aws.DeployScheduleRule(ctx, newRule)
	if err != nil {
		return err
	}
	if output.FailedEntryCount != 0 {
		log.Printf("[error] deploy schdule rule with failed entries %s", marshalJSONString(output.FailedEntries))
		return errors.New("failed entry count > 0")
	}
	log.Printf("[info] deploy schdule rule %s", *output.RuleArn)
	return nil
}
