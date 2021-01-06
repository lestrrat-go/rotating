package rotating_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/lestrrat-go/rotating"
	"github.com/stretchr/testify/assert"
)

type fakeClock struct {
	t time.Time
}

func NewFakeClock(t time.Time) *fakeClock {
	return &fakeClock{t: t}
}

func (c *fakeClock) Now() time.Time {
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.t = c.t.Add(d)
}

func (c *fakeClock) Set(t time.Time) {
	c.t = t
}

func TestMaxInterval(t *testing.T) {
	dir, err := ioutil.TempDir("", "rotating_test-MaxInterval")
	if !assert.NoError(t, err, `ioutil.TempDir should succeed`) {
		return
	}
	defer os.RemoveAll(dir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	linkName := filepath.Join(dir, "max-interval.log")
	clock := NewFakeClock(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))
	f, err := rotating.NewFile(
		ctx,
		filepath.Join(dir, "%Y%m%d-%H%M%S.log"),
		rotating.WithClock(clock),
		rotating.WithMaxInterval(5*time.Second),
		rotating.WithSymlink(linkName),
	)
	if !assert.NoError(t, err, `rotating.NewFile should succeed`) {
		return
	}

	expected := []string{
		"20210101-000000.log",
		"20210101-000005.log",
		"max-interval.log",
	}

	assertSymlink := func(t *testing.T, expected string) bool {
		t.Helper()

		sym, err := os.Readlink(linkName)
		if !assert.NoError(t, err, `os.Readlink should succeed`) {
			return false
		}
		if !assert.Equal(t, expected, sym) {
			return false
		}
		return true
	}

	const msg = "Hello, World\n"
	fmt.Fprintf(f, msg)
	if !assertSymlink(t, expected[0]) {
		return
	}

	clock.Advance(6 * time.Second)
	fmt.Fprintf(f, msg)
	f.Close()
	if !assertSymlink(t, expected[1]) {
		return
	}

	entries, err := os.ReadDir(dir)
	if !assert.NoError(t, err, `os.ReadDir should succeed`) {
		return
	}

	if !assert.Len(t, entries, 3, "should be 3 entries in directory") {
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for i, ent := range entries {
		t.Logf("found file(%d): %s", i, ent.Name())
		if !assert.Equal(t, expected[i], ent.Name()) {
			return
		}

		f, err := os.Open(filepath.Join(dir, ent.Name()))
		if !assert.NoError(t, err, `os.Open should succeed`) {
			return
		}
		defer f.Close()

		buf, err := ioutil.ReadAll(f)
		if !assert.NoError(t, err, `ioutil.ReadFull should succeed`) {
			return
		}

		if !assert.Equal(t, msg, string(buf), `contents should match for %s`, filepath.Join(dir, ent.Name())) {
			return
		}
	}
}

func TestMaxFileSize(t *testing.T) {
	dir, err := ioutil.TempDir("", "rotating_test-MaxFileSize")
	if !assert.NoError(t, err, `ioutil.TempDir should succeed`) {
		return
	}
	defer os.RemoveAll(dir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	clock := NewFakeClock(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))
	f, err := rotating.NewFile(
		ctx,
		filepath.Join(dir, "%Y%m%d-%H%M%S.log"),
		rotating.WithClock(clock),
		rotating.WithMaxFileSize(10),
		rotating.WithCheckInterval(100*time.Millisecond),
	)
	if !assert.NoError(t, err, `rotating.NewFile should succeed`) {
		return
	}

	fmt.Fprintf(f, "0123456789\n")
	time.Sleep(200 * time.Millisecond)
	fmt.Fprintf(f, "0123456789\n")
	entries, err := os.ReadDir(dir)
	if !assert.NoError(t, err, `os.ReadDir should succeed`) {
		return
	}

	if !assert.Len(t, entries, 2, "should be 2 entries in directory") {
		return
	}
}

func TestRotationCount(t *testing.T) {
	dir, err := ioutil.TempDir("", "rotating_test-RotationCount")
	if !assert.NoError(t, err, `ioutil.TempDir should succeed`) {
		return
	}
	defer os.RemoveAll(dir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	clock := NewFakeClock(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))
	f, err := rotating.NewFile(
		ctx,
		filepath.Join(dir, "%Y%m%d-%H%M%S.log"),
		rotating.WithClock(clock),
		rotating.WithMaxFileSize(1),
		rotating.WithMaxInterval(5*time.Second),
		rotating.WithCheckInterval(100*time.Millisecond),
		rotating.WithRotationCount(5),
	)

	for i := 0; i < 20; i++ {
		fmt.Fprintf(f, "0123456789\n")
		time.Sleep(150*time.Millisecond)
		if i == 9 {
			clock.Advance(6*time.Second)
		}
	}

	
	entries, err := os.ReadDir(dir)
	if !assert.NoError(t, err, `os.ReadDir should succeed`) {
		return
	}

	if !assert.Len(t, entries, 5, "should be 5 entries in directory") {
		return
	}

	for i, ent := range entries {
		t.Logf("found file(%d): %s", i, ent.Name())
	}
}
