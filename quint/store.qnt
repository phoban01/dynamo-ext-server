// Model of one resource in the store: the counter, the event log, TTL
// deletes, and one watcher. spec/solas.md sections 3 and 4 give the
// rules.
module store {
  type Watcher = {
    // The version of the last delivered event. The watch starts at 0.
    last: int,
    delivered: Set[int],
    // False once a delivery holds a version not above an earlier one.
    ordered: bool,
    gone: bool,
    // 0: idle. 1: read the counter, value c.
    phase: int,
    c: int,
  }

  // The number of writes the model allows.
  const MAXW: int
  // The negative model sets this to skip the gap check of spec 4.3.
  const noGapCheck: bool

  var counter: int
  var log: Set[int]
  var w: Watcher

  action init = all {
    counter' = 0,
    log' = Set(),
    w' = { last: 0, delivered: Set(), ordered: true, gone: false, phase: 0, c: 0 },
  }

  // A write increments the counter and adds its event in one
  // transaction, spec 2.3 and 3.1.
  action write = all {
    counter < MAXW,
    counter' = counter + 1,
    log' = log.union(Set(counter + 1)),
    w' = w,
  }

  // DynamoDB deletes an expired event, late and in any order, spec 4.1.
  action expire = all {
    log.size() > 0,
    nondet v = log.oneOf()
    all {
      counter' = counter,
      log' = log.exclude(Set(v)),
      w' = w,
    },
  }

  // Each poll reads the counter first, spec 4.2.
  action pollRead = all {
    not(w.gone),
    w.phase == 0,
    counter' = counter,
    log' = log,
    w' = { ...w, phase: 1, c: counter },
  }

  // Then it queries the versions from last + 1 to c.
  action pollQuery = {
    val want = 1.to(MAXW).filter(v => v > w.last and v <= w.c)
    val found = want.intersect(log)
    val complete = want.subseteq(log)
    all {
      not(w.gone),
      w.phase == 1,
      counter' = counter,
      log' = log,
      if (complete or noGapCheck)
        w' = {
          ...w,
          phase: 0,
          last: w.c,
          delivered: w.delivered.union(found),
          ordered: w.ordered and found.forall(v => w.delivered.forall(u => u < v)),
        }
      else
        w' = { ...w, phase: 0, gone: true },
    }
  }

  action step = any { write, expire, pollRead, pollQuery }

  // Delivered versions strictly increase, spec 3.2 and 4.2.
  val monotonicRV = w.ordered

  // A watch delivers every version up to last, or it ends with 410 Gone,
  // spec 4.2 and 4.3.
  val watchComplete = w.gone
    or 1.to(MAXW).forall(v => (v <= w.last) == w.delivered.contains(v))

  val storeSafety = monotonicRV and watchComplete

  // Witnesses: a watch delivers events, and a watch ends with 410 Gone.
  val witnessDelivered = w.delivered.size() < 2
  val witnessGone = not(w.gone)
}

module store3 {
  import store(MAXW = 3, noGapCheck = false).*
}
