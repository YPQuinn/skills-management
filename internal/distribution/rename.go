package distribution

import "errors"

// errDestinationExists is the no-replace rename's destination-exists error.
var errDestinationExists = errors.New("destination already exists")
