package rotating

import (
	"time"

	"github.com/lestrrat-go/option"
)

type Option = option.Interface

type identClock struct{}
type identCheckInterval struct{}
type identMaxFileSize struct{}
type identMaxInterval struct{}
type identRotationCount struct{}
type identSymlink struct{}

// WithClock creates a new Option that sets a clock that the File
// object will use to determine the current time.
//
// By default rotating.Local, which returns the current time in the local
// time zone, is used. If you/ would rather use UTC, use rotating.UTC
// as the argument to this option, and pass it to the constructor.
func WithClock(c Clock) Option {
	return option.New(identClock{}, c)
}

func WithCheckInterval(v time.Duration) Option {
	return option.New(identCheckInterval{}, v)
}

func WithMaxFileSize(v int64) Option {
	return option.New(identMaxFileSize{}, v)
}

// WithMaxInterval specifies the time between creation of a new file
//
// Please note that this option does not necessarily mean "files will be
// created after this amount of time" which is may be a bit surprising.
//
// This flag specifies that we should partition the time in slots that
// are `v` duration each, and create a new file every time the time is
// divisible by `v`
//
// For example, consider the following scenario:
// * `v` is `time.Hour`
// * we're starting to write at 00:05:00 Jan 1, 2021
// * pattern for the file name is "%Y%m%d-%H%M%S.log"
//
// In this scenario, we're in the hourly time slot starting at
// 00:00:00 Jan 1, 2021, so the file name will be `20210101-000000.log`
// When we reach the next time slot, we start writing to a file
// named `20210101-0100000.log` (also note that %M %S are always 0,
// because the resolution is "hour"). We would be writing to the same file
// even if we started writing at 00:59:59 Jan 1, 2021, only to switch to
// writing a new file (20210101-010000.log) soon after.
//
// This behavior is mainly due to the fact that there is no portable
// way of finding out the creation time of a file across platforms,
// and we can only reliably switch target files based on the current time.
func WithMaxInterval(v time.Duration) Option {
	return option.New(identMaxInterval{}, v)
}

func WithSymlink(v string) Option {
	return option.New(identSymlink{}, v)
}

func WithRotationCount(v int) Option {
	return option.New(identRotationCount{}, v)
}
