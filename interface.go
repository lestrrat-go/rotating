package rotating

import "time"

type Clock interface {
	Now() time.Time
}
