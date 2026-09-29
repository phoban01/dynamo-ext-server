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

  // ---- Helpers ----

  pure def active(cl: Cluster): bool = cl.up and not(cl.left)

  // The holder's own view of its lease, spec 7.4.
  def localLive(c: str): bool = {
    val cl = s.clusters.get(c)
    active(cl) and cl.uid != 0 and cl.clock < cl.renewedAt + CFG.D - CFG.M
  }

  pure def names(ref: OptRef, claim: str): bool = match ref {
    | Held(r) => r.claim == claim
    | Free => false
  }

  pure def namesWith(ref: OptRef, claim: str, muid: int): bool = match ref {
    | Held(r) => r.claim == claim and r.muid == muid
    | Free => false
  }

  pure def refMuid(ref: OptRef): int = match ref {
    | Held(r) => r.muid
    | Free => 0
  }

  pure def refMember(ref: OptRef): str = match ref {
    | Held(r) => r.member
    | Free => ""
  }

  pure def isFree(ref: OptRef): bool = match ref {
    | Held(_) => false
    | Free => true
  }

  // Claims of cluster c that were bound under another member UID become
  // Lost, spec 6.5.
  def markLost(c: str, uid: int): str -> Claim =
    s.claims.keys().mapBy(k => {
      val cm = s.claims.get(k)
      if (CFG.owner.get(k) == c
          and (cm.phase == "Bound" or cm.phase == "Suspended")
          and cm.muid != uid)
        { ...cm, phase: "Lost" }
      else cm
    })

  // ---- Membership, spec 7.2, 7.3 ----

  // Join, or renew the existing Member after a controller start.
  action join(c: str): bool = {
    val cl = s.clusters.get(c)
    val mo = s.members.get(c)
    val rv = s.memberCounter + 1
    val uid = if (mo.exists) mo.uid else s.nextUid
    all {
      active(cl),
      cl.uid == 0,
      s' = {
        ...s,
        members: s.members.set(c, { exists: true, uid: uid, rv: rv }),
        memberCounter: rv,
        nextUid: if (mo.exists) s.nextUid else s.nextUid + 1,
        clusters: s.clusters.set(c, {
          ...cl,
          uid: uid,
          memberRv: rv,
          renewedAt: cl.clock,
          renewInFlight: false,
        }),
        claims: markLost(c, uid),
      },
    }
  }

  // The holder records S, the local time at which it sends the renew.
  action renewSend(c: str): bool = {
    val cl = s.clusters.get(c)
    all {
      active(cl),
      cl.uid != 0,
      not(cl.renewInFlight),
      s' = {
        ...s,
        clusters: s.clusters.set(c, {
          ...cl,
          renewInFlight: true,
          renewSentAt: cl.clock,
          renewRv: cl.memberRv,
        }),
      },
    }
  }

  // The renew lands on condition that the Member rv is unchanged.
  action renewLand(c: str): bool = {
    val cl = s.clusters.get(c)
    val mo = s.members.get(c)
    val mine = mo.exists and mo.uid == cl.uid
    val rv = s.memberCounter + 1
    all {
      active(cl),
      cl.renewInFlight,
      if (mine and mo.rv == cl.renewRv)
        s' = {
          ...s,
          members: s.members.set(c, { ...mo, rv: rv }),
          memberCounter: rv,
          clusters: s.clusters.set(c, {
            ...cl,
            renewInFlight: false,
            memberRv: rv,
            renewedAt: cl.renewSentAt,
          }),
        }
      else
        // 404 NotFound clears the UID, so the member joins again.
        // 409 Conflict reads the rv again.
        s' = {
          ...s,
          clusters: s.clusters.set(c, {
            ...cl,
            renewInFlight: false,
            uid: if (mine) cl.uid else 0,
            memberRv: mo.rv,
          }),
        },
    }
  }

  // A crash stops the cluster. Its claims stay in its etcd, and its bind
  // writes in flight can still land.
  action crash(c: str): bool = {
    val cl = s.clusters.get(c)
    all {
      active(cl),
      s' = { ...s, clusters: s.clusters.set(c, { ...cl, up: false }) },
    }
  }

  // The controller starts again with no memory of its lease or its sweeps.
  action recover(c: str): bool = {
    val cl = s.clusters.get(c)
    all {
      not(cl.up),
      not(cl.left),
      s' = {
        ...s,
        clusters: s.clusters.set(c, {
          ...cl,
          up: true,
          uid: 0,
          renewInFlight: false,
          draining: false,
          seen: CFG.clusters.mapBy(_ => NOSEEN),
          sweepPhase: 0,
        }),
      },
    }
  }

  action step = any {
    tick,
    nondet c = CFG.clusters.oneOf()
    any {
      join(c),
      renewSend(c),
      renewLand(c),
      crash(c),
      recover(c),
    },
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
