package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zoobzio/clockz"
)

// Job represents a scheduled task
type Job struct {
	ID       string
	Schedule Schedule
	Task     func() error
	Retries  int
	Backoff  time.Duration
}

// Schedule defines when a job should run
type Schedule interface {
	Next(from time.Time) time.Time
}

// DailySchedule runs at a specific time each day
type DailySchedule struct {
	Hour   int
	Minute int
}

func (s DailySchedule) Next(from time.Time) time.Time {
	next := time.Date(from.Year(), from.Month(), from.Day(), s.Hour, s.Minute, 0, 0, from.Location())
	if !next.After(from) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// IntervalSchedule runs at fixed intervals
type IntervalSchedule struct {
	Interval time.Duration
}

func (s IntervalSchedule) Next(from time.Time) time.Time {
	return from.Add(s.Interval)
}

// CronSchedule simulates cron-like scheduling
type CronSchedule struct {
	Minute int // -1 for any
	Hour   int // -1 for any
	Day    int // -1 for any
}

func (s CronSchedule) Next(from time.Time) time.Time {
	next := from.Add(time.Minute)
	
	for {
		if (s.Minute == -1 || next.Minute() == s.Minute) &&
		   (s.Hour == -1 || next.Hour() == s.Hour) &&
		   (s.Day == -1 || next.Day() == s.Day) {
			return next
		}
		next = next.Add(time.Minute)
	}
}

// Scheduler manages job execution
type Scheduler struct {
	clock    clockz.Clock
	jobs     map[string]*scheduledJob
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

type scheduledJob struct {
	job      *Job
	nextRun  time.Time
	timer    clockz.Timer
	running  bool
	failures int
}

// NewScheduler creates a scheduler with real clock
func NewScheduler() *Scheduler {
	return NewSchedulerWithClock(clockz.RealClock)
}

// NewSchedulerWithClock creates a scheduler with custom clock
func NewSchedulerWithClock(clock clockz.Clock) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		clock:  clock,
		jobs:   make(map[string]*scheduledJob),
		ctx:    ctx,
		cancel: cancel,
	}
}

// AddJob adds a job with a schedule
func (s *Scheduler) AddJob(id string, schedule Schedule, task func() error) {
	s.AddJobWithRetry(id, schedule, task, 0, 0)
}

// AddJobWithRetry adds a job with retry logic
func (s *Scheduler) AddJobWithRetry(id string, schedule Schedule, task func() error, retries int, backoff time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job := &Job{
		ID:       id,
		Schedule: schedule,
		Task:     task,
		Retries:  retries,
		Backoff:  backoff,
	}

	nextRun := schedule.Next(s.clock.Now())
	sj := &scheduledJob{
		job:     job,
		nextRun: nextRun,
	}

	s.jobs[id] = sj
	s.scheduleJob(sj)
}

// AddDaily adds a job that runs daily at specified time
func (s *Scheduler) AddDaily(timeStr string, task func() error) {
	var hour, minute int
	fmt.Sscanf(timeStr, "%d:%d", &hour, &minute)
	
	schedule := DailySchedule{Hour: hour, Minute: minute}
	s.AddJob(fmt.Sprintf("daily-%s", timeStr), schedule, task)
}

// AddInterval adds a job that runs at fixed intervals
func (s *Scheduler) AddInterval(interval time.Duration, task func() error) {
	schedule := IntervalSchedule{Interval: interval}
	s.AddJob(fmt.Sprintf("interval-%v", interval), schedule, task)
}

// scheduleJob sets up timer for next execution
func (s *Scheduler) scheduleJob(sj *scheduledJob) {
	if sj.timer != nil {
		sj.timer.Stop()
	}

	delay := sj.nextRun.Sub(s.clock.Now())
	if delay < 0 {
		delay = 0
	}

	sj.timer = s.clock.AfterFunc(delay, func() {
		s.executeJob(sj)
	})
}

// executeJob runs a job and handles retries
func (s *Scheduler) executeJob(sj *scheduledJob) {
	s.mu.Lock()
	if sj.running {
		s.mu.Unlock()
		return
	}
	sj.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			sj.running = false
			s.mu.Unlock()
		}()

		// Execute with retries
		var err error
		attempts := 0
		maxAttempts := sj.job.Retries + 1

		for attempts < maxAttempts {
			err = sj.job.Task()
			attempts++
			
			if err == nil {
				// Success - schedule next run
				s.mu.Lock()
				sj.failures = 0
				sj.nextRun = sj.job.Schedule.Next(s.clock.Now())
				s.scheduleJob(sj)
				s.mu.Unlock()
				return
			}

			// Failed - apply backoff if retrying
			if attempts < maxAttempts {
				backoff := sj.job.Backoff
				if backoff > 0 {
					// Exponential backoff
					for i := 1; i < attempts; i++ {
						backoff *= 2
					}
					s.clock.Sleep(backoff)
				}
			}
		}

		// All retries exhausted
		s.mu.Lock()
		sj.failures++
		// Still schedule next run despite failure
		sj.nextRun = sj.job.Schedule.Next(s.clock.Now())
		s.scheduleJob(sj)
		s.mu.Unlock()
	}()
}

// Start begins the scheduler
func (s *Scheduler) Start() {
	// Jobs are scheduled when added, nothing to do here
}

// Stop gracefully stops the scheduler
func (s *Scheduler) Stop() {
	s.cancel()
	
	s.mu.Lock()
	for _, sj := range s.jobs {
		if sj.timer != nil {
			sj.timer.Stop()
		}
	}
	s.mu.Unlock()

	s.wg.Wait()
}

// GetJobStatus returns current status of a job
func (s *Scheduler) GetJobStatus(id string) (nextRun time.Time, failures int, found bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sj, ok := s.jobs[id]
	if !ok {
		return time.Time{}, 0, false
	}

	return sj.nextRun, sj.failures, true
}

// MaintenanceWindow prevents jobs from running during maintenance
type MaintenanceWindow struct {
	Start time.Time
	End   time.Time
}

// SchedulerWithMaintenance extends Scheduler with maintenance windows
type SchedulerWithMaintenance struct {
	*Scheduler
	windows []MaintenanceWindow
}

// NewSchedulerWithMaintenance creates a scheduler that respects maintenance windows
func NewSchedulerWithMaintenance(clock clockz.Clock) *SchedulerWithMaintenance {
	return &SchedulerWithMaintenance{
		Scheduler: NewSchedulerWithClock(clock),
		windows:   make([]MaintenanceWindow, 0),
	}
}

// AddMaintenanceWindow adds a period where jobs won't run
func (s *SchedulerWithMaintenance) AddMaintenanceWindow(start, end time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.windows = append(s.windows, MaintenanceWindow{
		Start: start,
		End:   end,
	})
}

// IsInMaintenance checks if current time is in a maintenance window
func (s *SchedulerWithMaintenance) IsInMaintenance() bool {
	now := s.clock.Now()
	for _, window := range s.windows {
		if now.After(window.Start) && now.Before(window.End) {
			return true
		}
	}
	return false
}

// JobChain represents dependent jobs that run in sequence
type JobChain struct {
	scheduler *Scheduler
	jobs      []func() error
	onError   func(step int, err error)
}

// NewJobChain creates a chain of dependent jobs
func NewJobChain(scheduler *Scheduler) *JobChain {
	return &JobChain{
		scheduler: scheduler,
		jobs:      make([]func() error, 0),
	}
}

// Then adds a job to the chain
func (c *JobChain) Then(job func() error) *JobChain {
	c.jobs = append(c.jobs, job)
	return c
}

// OnError sets error handler for the chain
func (c *JobChain) OnError(handler func(step int, err error)) *JobChain {
	c.onError = handler
	return c
}

// Run executes the job chain
func (c *JobChain) Run() error {
	for i, job := range c.jobs {
		if err := job(); err != nil {
			if c.onError != nil {
				c.onError(i, err)
			}
			return fmt.Errorf("step %d failed: %w", i, err)
		}
	}
	return nil
}