# rotating

A representation of a file that knows how to rotate itself based on size/interval

# CAVEAT EMPTOR

This software has not been tested in the wild yet (Jan 2020)

# SYNOPSIS

```go
f, err := rotating.NewFile(
	ctx,
	filepath.Join(dir, "%Y%m%d-%H%M%S.log"),
	rotating.WithClock(clock),
	rotating.WithMaxFileSize(1),
	rotating.WithMaxInterval(5*time.Second),
	rotating.WithCheckInterval(100*time.Millisecond),
	rotating.WithRotationCount(5),
)

// *rotating.File fulfills io.Writer, so you can use it anywehere
// io.Writer is accepted
fmt.Fprintf(f, ...)
```

# CONTEXT

The first argument to `rotation.NewFile` is a context object. This context
should be kept alive during the entire time you use the log. It is used to
control certain (possibly) long-running operations inside the object.

# PATTERN

The second argument to `rotation.NewFile` is the file name pattern to use
to generate the backing files. Tha pettern is fed into 
[github.com/lestrrat-go/strftime](https://github.com/lestrrat-go/strftime).

## FILENAMES AND ROTATION

While you are free to configure your filenames as you please, when using
`rotating.WithRotateCount` option, you should make sure to include all of
the necessary components to figure out precisely when the files were generated.

This is because files to purge are determined by *file name order*. 
If you do not provide enough information in your file names to allow
sorting in chronologicall order, we will not be able to figure out
which files to purge. 

For example, if you provided a file name pattern like `%H%M%S.log`, we will
not be able to determine if files generated at the same time belong
to today or yesterday, or ..., and hence we might delete the wrong file.

Why can't we use the modification timestamp? We could, but there are
edge cases like a user updating a file manually, ore we *sync* the 
previous log *after* we start writing to the new log. And if the next
rotation happens fast enough, the timestamps may not aligh properly 
as expected.

While in reality this edge case will not trigger unless a specific
configuration is used, being able to explain that files are purged
in the file name order is much simpler for everybody, so we use that rule.

## TIMESTAMPS USED FOR FILENAMES

We use a "clock" to determine the time to switch log files, and not
elapsed times (via, for example, a time.Ticker). This is because
in reality system clocks can be changed during an execution of a
long running process, and sometimes you need to rotate files based on
those changes.

This also means that we need to be able to determine which files to
write by the current wall clock time. Therefore we use the observed
wall clock time and the maximum interval (via `WithMaxInterval`)

At a given time `t`, we truncate the time by the interval, and use
that truncated time to generate the current file name that we think
we should write to.

For example, given an interval of one hour, and a file name pattern
of `%H%M%S.log`, the following rules apply:

| time                | filename   |
|---------------------|------------|
| 2021-01-01 00:00:00 | 000000.log |
| 2021-01-01 00:59:00 | 000000.log |
| 2021-01-01 01:00:00 | 010000.log |
| 2021-01-01 23:01:00 | 230100.log |

# BUFFERING

The underlying `io.Writer` for `*rotating.File` is a raw `*os.File`.
Therefore to maximize efficiency you should wrap the object in a `bufio.Writer`

# OPTIONS

## WithMaxInterval(time.Duration)

Specifies the interval between switching log files.

## WithMaxFileSize(int64)

Specifies the max file size before switching log files.

## WithRotationCount(int)

Specifies the number of logs to retain. See the `PATTERN` for an
explanation of how the files to retain are selected.

## WithSymlink(string)

Creates a symlink to the current log file being written to.

## WithClock(Clock)

Use to provide a Clock to the file. For example, 

# Filing Issues

Please do not file issues without code to show for it. Issues labeled with
"needs reproduction" and without a minimal reproducible standalone Go
test case may be closed without any warning.
