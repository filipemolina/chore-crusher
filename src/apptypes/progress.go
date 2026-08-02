package apptypes

// DerivedPercent computes the derived progress percentage for a task with
// ProgressSubtasks mode without a DB query. It scans the already-flattened
// rows looking for direct children and computes their complete/total ratio.
// Returns displayAsSimple true when there are no children (progress should
// display as simple, not a percent).
func DerivedPercent(rows []Row, taskID string) (percent int, displayAsSimple bool) {
	var complete, total int
	for _, row := range rows {
		if row.Task.ParentID != nil && *row.Task.ParentID == taskID {
			total++
			if row.Task.Status == StatusComplete {
				complete++
			}
		}
	}
	if total == 0 {
		return 0, true
	}
	pct := int(float64(complete)*100+0.5) / total // round(complete * 100 / total)
	return pct, false
}
