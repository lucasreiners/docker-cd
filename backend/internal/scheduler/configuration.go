package scheduler

import (
	"github.com/robfig/cron/v3"
	"time"
)

func (s *SchedulerService) EffectiveSchedule() (string, string, time.Time) {
	if s == nil {
		return "", "", time.Time{}
	}
	schedule, err := cron.ParseStandard(s.config.UpdaterCron)
	if err != nil {
		return s.config.UpdaterCron, time.Local.String(), time.Time{}
	}
	return s.config.UpdaterCron, time.Local.String(), schedule.Next(time.Now())
}
