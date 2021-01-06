package rotating

import "time"

type ClockFn func() time.Time

func (fn ClockFn) Now() time.Time {
	return fn()
}

func utc() time.Time {
	return time.Now().UTC()
}

func UTC() Clock {
	return ClockFn(utc)
}

func local() time.Time {
	return time.Now().Local()
}

func Local() Clock {
	return ClockFn(local)
}

func truncate(t time.Time, interval time.Duration) time.Time {
	// XXX HACK: Truncate only happens in UTC semantics, apparently.
	// observed values for truncating given time with 86400 secs:
	//
	// before truncation: 2018/06/01 03:54:54 2018-06-01T03:18:00+09:00
	// after  truncation: 2018/06/01 03:54:54 2018-05-31T09:00:00+09:00
	//
	// This is really annoying when we want to truncate in local time
	// so we hack: we take the apparent local time in the local zone,
	// and pretend that it's in UTC. do our math, and put it back to
	// the local zone
	if t.Location() == time.UTC {
		t = t.Truncate(interval)
	} else {
		// Pretend that we're in UTC
		utc := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)

		// Do the truncation while we're in UTC
		utc = utc.Truncate(interval)

		// Now use them values and put them back into our original location
		t = time.Date(utc.Year(), utc.Month(), utc.Day(), utc.Hour(), utc.Minute(), utc.Second(), utc.Nanosecond(), t.Location())
	}
	return t
}
