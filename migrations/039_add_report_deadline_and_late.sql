-- Monthly report lifecycle: record whether a report was submitted after the
-- period's reporting window closed but before its (now mandatory) submission
-- deadline, so "逾期提交" is retained through review.
ALTER TABLE reports ADD COLUMN IF NOT EXISTS late_submission BOOLEAN NOT NULL DEFAULT FALSE;
