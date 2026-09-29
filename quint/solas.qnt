// Model of the solas protocol: members, devices, claims, and the sweeper.
// spec/solas.md sections 5 to 8 give the rules. The store is a
// linearizable map with conditional writes, as sections 2 and 3 give.
module solas {
  // A claimRef, spec 5.2. The claim ID stands for the claim UID.
  type ClaimRef = { member: str, muid: int, claim: str }
  type OptRef = Held(ClaimRef) | Free

  type Device = { rv: int, ref: OptRef }

  // A Member object in the table, spec 7.1.
  type MemberObj = { exists: bool, uid: int, rv: int }

  // A DeviceClaim in the owner's etcd, spec 6.1.
  type Claim = {
    exists: bool,
    deleting: bool,
    phase: str,
    dev: str,
    muid: int,
  }

  // What an observer saw of another Member, spec 8.1. rv -1 means none.
  type Seen = { rv: int, at: int }

  // The local state of one member cluster and its controller.
  type Cluster = {
    up: bool,
    left: bool,
    clock: int,
    uid: int,
    memberRv: int,
    renewedAt: int,
    renewInFlight: bool,
    renewSentAt: int,
    renewRv: int,
    draining: bool,
    seen: str -> Seen,
    sweepPhase: int,
    sweepDevs: str -> Device,
    sweepUids: Set[int],
  }

  // A bind write in flight from a controller to the table.
  type BindMsg = { cluster: str, claim: str, dev: str, rv: int, muid: int }

  type Config = {
    clusters: Set[str],
    devices: Set[str],
    claims: Set[str],
    owner: str -> str,
    clock0: str -> int,
    D: int,
    M: int,
    tick: int,
    drift: int,
    // Flags that break one rule each, for the negative models.
    unconditionalBind: bool,
    membersFirst: bool,
    adoptAnyUid: bool,
  }

  type State = {
    now: int,
    devices: str -> Device,
    devCounter: int,
    members: str -> MemberObj,
    memberCounter: int,
    nextUid: int,
    clusters: str -> Cluster,
    claims: str -> Claim,
    inflight: Set[BindMsg],
    // Ghost flags for the witnesses.
    cleared: bool,
    someLeft: bool,
  }

  const CFG: Config

  var s: State

  pure val NOSEEN: Seen = { rv: -1, at: 0 }

  pure def initCluster(cfg: Config, c: str): Cluster = {
    up: true,
    left: false,
    clock: cfg.clock0.get(c),
    uid: 0,
    memberRv: 0,
    renewedAt: 0,
    renewInFlight: false,
    renewSentAt: 0,
    renewRv: 0,
    draining: false,
    seen: cfg.clusters.mapBy(_ => NOSEEN),
    sweepPhase: 0,
    sweepDevs: cfg.devices.mapBy(_ => { rv: 0, ref: Free }),
    sweepUids: Set(),
  }

  action init = s' = {
    now: 0,
    devices: CFG.devices.mapBy(_ => { rv: 0, ref: Free }),
    devCounter: 0,
    members: CFG.clusters.mapBy(_ => { exists: false, uid: 0, rv: 0 }),
    memberCounter: 0,
    nextUid: 1,
    clusters: CFG.clusters.mapBy(c => initCluster(CFG, c)),
    claims: CFG.claims.mapBy(_ => {
      exists: true, deleting: false, phase: "Pending", dev: "", muid: 0,
    }),
    inflight: Set(),
    cleared: false,
    someLeft: false,
  }

  // ---- Time, spec 7.5 ----

  // Real time moves by one tick. Each clock moves by one tick plus or
  // minus the drift, so the rate drift bound rho is drift / tick.
  action tick = {
    nondet adv = setOfMaps(CFG.clusters, Set(-CFG.drift, 0, CFG.drift)).oneOf()
    s' = {
      ...s,
      now: s.now + CFG.tick,
      clusters: s.clusters.keys().mapBy(c => {
        ...s.clusters.get(c),
        clock: s.clusters.get(c).clock + CFG.tick + adv.get(c),
      }),
    }
  }

  action step = any {
    tick,
  }
}

// Two clusters, two devices, and three claims. The clocks start far
// apart, because solas does not depend on synchronized clocks.
module solas2 {
  import solas(CFG = {
    clusters: Set("a", "b"),
    devices: Set("d1", "d2"),
    claims: Set("a1", "a2", "b1"),
    owner: Map("a1" -> "a", "a2" -> "a", "b1" -> "b"),
    clock0: Map("a" -> 0, "b" -> 1000),
    D: 30,
    M: 6,
    tick: 10,
    drift: 1,
    unconditionalBind: false,
    membersFirst: false,
    adoptAnyUid: false,
  }).*
}
