//go:build no_logs

package categraf

import "flashcat.cloud/categraf/types"

func gatherLogMetrics(slist *types.SampleList) {}
