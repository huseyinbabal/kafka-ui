-- Analytics samples. *_raw keeps every collector tick (default 5s) for a few
-- hours; *_1m keeps one row per minute for the retention period. Offsets are
-- sums over a topic's partitions; rates are derived from deltas at query time.
CREATE TABLE topic_samples_raw (
  cluster    TEXT    NOT NULL,
  topic      TEXT    NOT NULL,
  ts         INTEGER NOT NULL, -- unix millis
  end_sum    INTEGER NOT NULL,
  start_sum  INTEGER NOT NULL,
  size_bytes INTEGER NOT NULL, -- -1 when unknown
  PRIMARY KEY (cluster, topic, ts)
) WITHOUT ROWID;
CREATE TABLE topic_samples_1m (
  cluster    TEXT    NOT NULL,
  topic      TEXT    NOT NULL,
  ts         INTEGER NOT NULL,
  end_sum    INTEGER NOT NULL,
  start_sum  INTEGER NOT NULL,
  size_bytes INTEGER NOT NULL,
  PRIMARY KEY (cluster, topic, ts)
) WITHOUT ROWID;

CREATE TABLE group_samples_raw (
  cluster       TEXT    NOT NULL,
  grp           TEXT    NOT NULL,
  topic         TEXT    NOT NULL,
  ts            INTEGER NOT NULL,
  committed_sum INTEGER NOT NULL, -- over partitions with a commit
  lag           INTEGER NOT NULL,
  PRIMARY KEY (cluster, grp, topic, ts)
) WITHOUT ROWID;
CREATE TABLE group_samples_1m (
  cluster       TEXT    NOT NULL,
  grp           TEXT    NOT NULL,
  topic         TEXT    NOT NULL,
  ts            INTEGER NOT NULL,
  committed_sum INTEGER NOT NULL,
  lag           INTEGER NOT NULL, -- max within the minute
  PRIMARY KEY (cluster, grp, topic, ts)
) WITHOUT ROWID;

CREATE TABLE cluster_samples_raw (
  cluster    TEXT    NOT NULL,
  ts         INTEGER NOT NULL,
  partitions INTEGER NOT NULL,
  urp        INTEGER NOT NULL,
  offline    INTEGER NOT NULL,
  PRIMARY KEY (cluster, ts)
) WITHOUT ROWID;
CREATE TABLE cluster_samples_1m (
  cluster    TEXT    NOT NULL,
  ts         INTEGER NOT NULL,
  partitions INTEGER NOT NULL,
  urp        INTEGER NOT NULL, -- max within the minute
  offline    INTEGER NOT NULL,
  PRIMARY KEY (cluster, ts)
) WITHOUT ROWID;
