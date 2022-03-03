/*
 * Banner Bard: Banner-serving discord bot, sire.
 *
 * scheduler.go - Tag scheduler. This manages setting the banner to a
 * tag over time, driving the `shuffle`, `cycle`, and `play` commands
 * (and their playlist variants). The heart of the scheduler is
 * StartJob() -- I recommend familiarizing yourself with that first.
 *
 *
 * This program uses the BSD 3-Clause license. You can find details under
 * the file LICENSE or under <https://opensource.org/licenses/BSD-3-Clause>.
 */
package main

import (
	"github.com/bwmarrin/discordgo"
	"math/rand"
	"time"
)

type BannerPicker interface {
	// Attempt to pick a tag. An empty string means to stop the scheduler.
	pickTag(tags []string) string

	// Notify that the pick was successful, and change any state
	// required to prepare for picking the next tag.
	success()
}

type ShufflePicker struct{}

type CyclePicker struct {
	index int
}

type OnceonlyPicker struct {
	index int
}

type BannerScheduler struct {
	session  *discordgo.Session
	tags     []string
	interval time.Duration
	picker   BannerPicker
	chnl     chan int
	active   bool
}

const (
	TimerReset = iota
	TimerStop
)

// Banner Pickers. These decide what the next tag should be, or
// whether to stop displaying tags altogether.

func (picker *ShufflePicker) pickTag(tags []string) string {
	return tags[rand.Intn(len(tags))]
}

func (picker *ShufflePicker) success() {}

func ScheduleShuffle() BannerPicker {
	return new(ShufflePicker)
}

//
func (picker *CyclePicker) pickTag(tags []string) string {
	if len(tags) <= picker.index {
		picker.index = 0
	}

	return tags[picker.index]
}

func (picker *CyclePicker) success() {
	picker.index++
}

func ScheduleCycle() BannerPicker {
	return new(CyclePicker)
}

//
func (picker *OnceonlyPicker) pickTag(tags []string) string {
	if len(tags) <= picker.index {
		return ""
	} else {
		return tags[picker.index]
	}
}

func (picker *OnceonlyPicker) success() {
	picker.index++
}

func ScheduleOnceonly() BannerPicker {
	return new(OnceonlyPicker)
}

// The Scheduler

func NewScheduler(s *discordgo.Session) *BannerScheduler {
	return &BannerScheduler{
		session: s,
		chnl:    make(chan int),
	}
}

/*
 * Start the tag scheduler. This procedure lasts forever, so call it
 * with `go` to launch the scheduler in the background.
 */
func (scheduler *BannerScheduler) StartJob(s *discordgo.Session) *BannerScheduler {
	scheduler.session = s
	// Allocate a ticker and stop it immediately, so that
	// accessing ticker.C initially doesn't raise a segfault.
	ticker := time.NewTicker(time.Hour)
	ticker.Stop()

	for {
		select {
		case <-ticker.C:
			logger.Println("Next banner")
			scheduler.Next()
		case action := <-scheduler.chnl:
			switch action {
			case TimerReset:
				// The scheduler has been updated with
				// new state, update the timer to
				// reflect the changes.
				scheduler.active = true
				ticker.Stop()
				ticker = time.NewTicker(scheduler.interval)

				// start the first banner
				scheduler.Next()
			case TimerStop:
				scheduler.active = false
				logger.Println("TimerStop")
				ticker.Stop()
			default:
				logger.Printf("Unknown scheduler value %d\n", action)
			}
		}
	}
}

func remove(slice []string, test string) []string {
	for i, item := range slice {
		if test == item {
			return append(slice[:i], slice[i+1:]...)
		}
	}

	return slice
}

func (scheduler *BannerScheduler) pickTag() string {
	return scheduler.picker.pickTag(scheduler.tags)
}

/*
 * Set the next tag.
 */
func (scheduler *BannerScheduler) Next() bool {
	if !scheduler.active {
		return false
	}

	// Pick a tag
	tag := scheduler.pickTag()
	if tag == "" {
		logger.Println("Banner picker gave nothing; stopping scheduler")
		scheduler.Stop()
		return true
	}

	// If the tag doesn't exist (deleted while cycling), readjust
	// the tag list and try again.
	for exists, err := tagExists(tag); !exists || err != nil; {
		// Take the tag out
		scheduler.tags = remove(scheduler.tags, tag)
		if len(scheduler.tags) == 0 {
			logger.Println("Banner picker gave nothing; stopping scheduler")
			scheduler.Stop()
			return true
		}

		tag = scheduler.pickTag()
	}
	scheduler.picker.success()

	err := setBanner(scheduler.session, tag)
	if err != nil {
		logger.Println("Error while setting the banner: " + err.Error())
	}

	return true
}

/*
 * Stop the scheduler
 */
func (scheduler *BannerScheduler) Stop() bool {
	wasActive := scheduler.active
	scheduler.chnl <- TimerStop

	return wasActive
}

/*
 * Set the tag schedule, including the interval between tags, the tags
 * themselves, and the picker used to decide how to choose each next
 * tag.
 */
func (scheduler *BannerScheduler) Set(interval time.Duration, tags []string,
	pickerProducer func() BannerPicker) (valid bool, err error) {

	// Stop the scheduler for now as we're setting up the state.
	scheduler.Stop()
	scheduler.picker = pickerProducer()

	if len(tags) == 0 {
		// An empty tag list is invalid
		return false, nil
	}

	for _, tag := range tags {
		// Nonexisting tags are also invalid
		ok, err := tagExists(tag)
		if !ok || err != nil {
			return false, err
		}
	}

	scheduler.interval = interval
	scheduler.tags = tags
	scheduler.chnl <- TimerReset
	return true, nil
}
