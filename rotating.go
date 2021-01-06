// Package rotating provides tools to write to a "self-rotating" file.
// Unlike using an external service like logrotate, the File struct
// knows how to rotate itself when the conditions are met.
package rotating

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/backoff"
	"github.com/lestrrat-go/strftime"
	"github.com/pkg/errors"
)

type File struct {
	backoff       backoff.Policy
	baseTime      time.Time
	cancel        func()
	checkInterval time.Duration
	clock         Clock
	ctx           context.Context
	file          io.Writer
	filename      string // current filename
	generation    int
	globPattern   string
	pattern       *strftime.Strftime
	lastCheck     time.Time
	maxAge        time.Duration
	maxInterval   time.Duration
	maxFileSize   int64
	mu            sync.RWMutex
	nextCheck     *time.Timer
	rotationCount int
	symlink       string
}

const (
	defaultCheckInterval = 5 * time.Minute
)

var patternConversionRegexps = []*regexp.Regexp{
	regexp.MustCompile(`%[%+A-Za-z]`),
	regexp.MustCompile(`\*+`),
}

func NewFile(ctx context.Context, p string, options ...Option) (*File, error) {
	bo := backoff.Null()
	clock := Local()
	maxInterval := time.Hour
	var checkInterval time.Duration
	var maxFileSize int64 = 0
	var symlink string
	var rotationCount int
	for _, option := range options {
		switch option.Ident() {
		case identClock{}:
			clock = option.Value().(Clock)
		case identCheckInterval{}:
			checkInterval = option.Value().(time.Duration)
		case identMaxInterval{}:
			maxInterval = option.Value().(time.Duration)
		case identMaxFileSize{}:
			maxFileSize = option.Value().(int64)
		case identSymlink{}:
			symlink = option.Value().(string)
		case identRotationCount{}:
			rotationCount = option.Value().(int)
		}
	}

	// Create the basic strftime pattern object to generate the filenames
	pattern, err := strftime.New(p)
	if err != nil {
		return nil, errors.Wrap(err, `invalid strftime pattern`)
	}

	if maxFileSize > 0 && checkInterval <= 0 {
		checkInterval = defaultCheckInterval
	}

	// Create the timer to periodically check for the file state
	var nextCheck *time.Timer
	if checkInterval > 0 {
		nextCheck = time.NewTimer(checkInterval)
	} else {
		nextCheck = time.NewTimer(0)
		if !nextCheck.Stop() {
			select {
			case <-nextCheck.C:
			default:
			}
		}
	}

	// Create a glob pattern so that we can purge old files
	globPattern := p
	for _, re := range patternConversionRegexps {
		globPattern = re.ReplaceAllString(globPattern, "*")
	}
	if !strings.HasSuffix(globPattern, "*") {
		globPattern = globPattern + "*" // allow suffixes
	}

	wctx, cancel := context.WithCancel(ctx)
	f := &File{
		backoff:       bo,
		ctx:           wctx,
		cancel:        cancel,
		checkInterval: checkInterval,
		clock:         clock,
		globPattern:   globPattern,
		maxFileSize:   maxFileSize,
		maxInterval:   maxInterval,
		nextCheck:     nextCheck,
		pattern:       pattern,
		rotationCount: rotationCount,
		symlink:       symlink,
	}

	return f, nil
}

func (f *File) Close() error {
	f.cancel()
	if f.file != nil {
		finalizeWriter(f.file)
	}
	return nil
}

func (f *File) sizeExceeded() bool {
	f.mu.Lock()
	var checkSize bool
	select {
	// Don't check for sizes in every single Write() call
	case <-f.nextCheck.C:
		checkSize = true
		f.nextCheck.Reset(f.checkInterval)
	default:
	}
	f.mu.Unlock()

	if !checkSize {
		return false
	}

	f.mu.RLock()
	if f.file == nil {
		f.mu.RUnlock()
		return false
	}
	flushWriter(f.file)
	maxFileSize := f.maxFileSize
	// XXX DO NOT USE (*os.File).Stat() here. Always use os.Stat(filename)
	// otherwise you will not be able to detect, for example, the file
	// missing in the file system
	fi, err := os.Stat(f.filename)
	f.mu.RUnlock()

	if err != nil {
		// if we couldn't stat... well, it could be because of a gazillion reasons
		// but one thing we can handle for sure is the file missing
		if os.IsNotExist(err) {
			return true // size hasn't exceeded, but...
		}
		// Play it safe otherwise
		return false
	}

	// Do we have a maximum size that we need to rotate by?
	return maxFileSize >= 0 && fi.Size() >= maxFileSize
}

func (f *File) intervalExceeded() bool {
	return !f.baseTime.Equal(truncate(f.clock.Now(), f.maxInterval))
}

func flushWriter(w io.Writer) {
	if v, ok := w.(interface{ Flush() error }); ok {
		_ = v.Flush()
	}

	if v, ok := w.(interface{ Sync() error }); ok {
		_ = v.Sync()
	}
}

func finalizeWriter(w io.Writer) {
	flushWriter(w)
	if v, ok := w.(io.Closer); ok {
		_ = v.Close()
	}
}

func (f *File) rotateFile(ctx context.Context, newFileName string) error {
	var lastError error
	// attempt to open new file. try for a bit
	f.mu.RLock()
	b := f.backoff.Start(ctx)
	f.mu.RUnlock()

	for backoff.Continue(b) {
		newF, err := createFile(newFileName)
		if err != nil {
			lastError = err
			continue
		}

		// created new file. assign it to the cache, and flush the previous
		// file. Closing the previous file is done asynchronously
		f.mu.Lock()
		if f.file != nil {
			finalizeWriter(f.file)
		}
		f.file = &bufferedWriter{
			Writer:     bufio.NewWriter(newF),
			baseWriter: newF,
		}
		f.filename = newFileName
		f.mu.Unlock()

		if err := f.makeSymlink(); err != nil {
			return errors.Wrap(err, `failed to create symlink`)
		}

		if err := f.purgeOld(); err != nil {
		}

		return nil
	}

	return errors.Wrapf(lastError, `failed to create file %s`, newFileName)
}

func (f *File) makeSymlink() error {
	sym := f.symlink
	if sym == "" {
		return nil
	}

	lockFn := f.filename + `_lock`
	fh, err := os.OpenFile(lockFn, os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return errors.Wrap(err, `failed to open lockfile`)
	}
	defer func() {
		_ = fh.Close()
		_ = os.Remove(lockFn)
	}()

	// Change how the link name is generated based on where the
	// target location is. if the location is directly underneath
	// the main filename's parent directory, then we create a
	// symlink with a relative path
	linkDst := f.filename
	linkDir := filepath.Dir(f.symlink)
	if strings.Contains(linkDst, linkDir) {
		tmp, err := filepath.Rel(linkDir, linkDst)
		if err != nil {
			return errors.Wrapf(err, `failed to evaluate relative path from %#v to %#v`, linkDir, linkDst)
		}
		linkDst = tmp
	}

	if _, err := os.Stat(linkDir); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(linkDir, 0755); err != nil {
			return errors.Wrapf(err, `failed to create directory %s`, linkDir)
		}
	}

	linkFn := f.filename + `_symlink`
	if err := os.Symlink(linkDst, linkFn); err != nil {
		return errors.Wrap(err, `failed to create symlink`)
	}

	if err := os.Rename(linkFn, f.symlink); err != nil {
		return errors.Wrap(err, `failed to rename new symlink`)
	}
	return nil
}

type bufferedWriter struct {
	*bufio.Writer
	baseWriter interface {
		io.WriteCloser
		Sync() error
	}
}

func (w *bufferedWriter) Sync() error {
	return w.baseWriter.Sync()
}

func (w *bufferedWriter) Close() error {
	return w.baseWriter.Close()
}

// Write satisfies the io.Writer interface.
//
func (f *File) Write(p []byte) (int, error) {
	w, err := f.getWriter()
	if err != nil {
		return 0, errors.Wrap(err, `failed to obtain file handle`)
	}

	return w.Write(p)
}

func (f *File) getWriter() (io.Writer, error) {
	sizeExceeded := f.sizeExceeded()
	intervalExceeded := f.intervalExceeded()
	if sizeExceeded || intervalExceeded {
		f.baseTime = truncate(f.clock.Now(), f.maxInterval)
		fn := f.pattern.FormatString(f.baseTime)
		if intervalExceeded {
			f.generation = 0
		} else {
			if fn == f.filename { // We are still writing to the same "time slot"
				f.generation++
				fn = fmt.Sprintf("%s.%d", fn, f.generation)
			}
		}

		if err := f.rotateFile(f.ctx, fn); err != nil {
			return nil, errors.Wrap(err, `failed to rotate file`)
		}
	}
	return f.file, nil
}

// createFile creates a new file in the given path, creating parent directories
// as necessary
func createFile(filename string) (*os.File, error) {
	// make sure the dir is existed, eg:
	// ./foo/bar/baz/hello.log must make sure ./foo/bar/baz is existed
	dirname := filepath.Dir(filename)
	if _, err := os.Stat(dirname); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(dirname, 0755); err != nil {
				return nil, errors.Wrapf(err, "failed to create directory %s", dirname)
			}
		}
	}

	// if we got here, then we need to create a file
	fh, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, errors.Errorf("failed to open file %s: %s", filename, err)
	}

	return fh, nil
}

func (f *File) purgeOld() error {
	matches, err := filepath.Glob(f.globPattern)
	if err != nil {
		return errors.Wrap(err, `failed to apply glob pattern`)
	}

	stats := make(map[string]os.FileInfo)
	// stat all the files once and cache
	for _, path := range matches {
		// Ignore temporary files
		if strings.HasSuffix(path, "_lock") || strings.HasSuffix(path, "_symlink") {
			continue
		}

		fi, err := os.Lstat(path)
		if err != nil {
			continue
		}

		stats[path] = fi
	}

	var protected bool
	if sym := f.symlink; sym != "" {
		// If we have a symlink and that symlink points to one of the
		// files that is a candidate to be deleted... do NOT delete it
		dst, err := os.Readlink(sym)
		if err == nil {
			delete(stats, dst)
			// remember that we have one extra file, so that we can
			// use that in the calculation of rotationCount
			protected = true
		}
	}

	matches = make([]string, 0, len(stats))
	for path := range stats {
		matches = append(matches, path)
	}

	// sort by name.
	sort.Slice(matches, func(i, j int) bool {
		return strings.Compare(matches[i], matches[j]) < 0
	})

	maxAge := f.maxAge
	toPurge := make([]string, 0, len(matches))
	candidates := make([]string, 0, len(matches))

	cutoff := f.clock.Now().Add(-1 * maxAge)
	for _, path := range matches {

		fi, ok := stats[path]
		if !ok {
			continue
		}

		if maxAge > 0 && fi.ModTime().After(cutoff) {
			toPurge = append(toPurge, path)
			continue
		}

		if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
			continue
		}

		candidates = append(candidates, path)
	}

	if c := f.rotationCount; c > 0 {
		// if we protected a file from being deleted, we need to add 1
		// to the total count of files
		lc := len(candidates)
		if protected {
			c--
		}
		if lc > c {
			toPurge = append(toPurge, candidates[:lc-c]...)
		}
	}

	if len(toPurge) > 0 {
		// Finally, start removing the files
		go func(files []string) {
			for _, file := range files {
				_ = os.Remove(file)
			}
		}(toPurge)
	}

	return nil
}
