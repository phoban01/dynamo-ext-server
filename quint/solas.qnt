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

  // ---- Sweeper, spec 8 ----

  // The observer records the local time at which it first saw the current
  // version of another Member, spec 8.1.
  action observe(c: str, m: str): bool = {
    val cl = s.clusters.get(c)
    val mo = s.members.get(m)
    val now = if (mo.exists) { rv: mo.rv, at: cl.clock } else NOSEEN
    all {
      active(cl),
      m != c,
      cl.seen.get(m).rv != now.rv,
      s' = {
        ...s,
        clusters: s.clusters.set(c, { ...cl, seen: cl.seen.set(m, now) }),
      },
    }
  }

  // Delete an expired Member with the observed rv, spec 8.2.
  action sweepDelete(c: str, m: str): bool = {
    val cl = s.clusters.get(c)
    val seen = cl.seen.get(m)
    val mo = s.members.get(m)
    val rv = s.memberCounter + 1
    all {
      active(cl),
      m != c,
      seen.rv >= 0,
      cl.clock >= seen.at + CFG.D,
      mo.exists,
      mo.rv == seen.rv,
      s' = {
        ...s,
        members: s.members.set(m, { exists: false, uid: mo.uid, rv: rv }),
        memberCounter: rv,
      },
    }
  }

  pure def liveUids(members: str -> MemberObj): Set[int] =
    members.keys().filter(m => members.get(m).exists).map(m => members.get(m).uid)

  // The sweeper lists devices, then members, spec 8.3. The membersFirst
  // flag swaps the order.
  action sweepListDevices(c: str): bool = {
    val cl = s.clusters.get(c)
    val from = if (CFG.membersFirst) 1 else 0
    all {
      active(cl),
      cl.sweepPhase == from,
      s' = {
        ...s,
        clusters: s.clusters.set(c, {
          ...cl,
          sweepPhase: cl.sweepPhase + 1,
          sweepDevs: s.devices,
        }),
      },
    }
  }

  action sweepListMembers(c: str): bool = {
    val cl = s.clusters.get(c)
    val from = if (CFG.membersFirst) 0 else 1
    all {
      active(cl),
      cl.sweepPhase == from,
      s' = {
        ...s,
        clusters: s.clusters.set(c, {
          ...cl,
          sweepPhase: cl.sweepPhase + 1,
          sweepUids: liveUids(s.members),
        }),
      },
    }
  }

  // Clear a claimRef whose member UID is not in the member list. The clear
  // carries the rv from the device list.
  action sweepClear(c: str, d: str): bool = {
    val cl = s.clusters.get(c)
    val listed = cl.sweepDevs.get(d)
    val rv = s.devCounter + 1
    all {
      active(cl),
      cl.sweepPhase == 2,
      not(isFree(listed.ref)),
      not(cl.sweepUids.contains(refMuid(listed.ref))),
      s.devices.get(d).rv == listed.rv,
      s' = {
        ...s,
        devices: s.devices.set(d, { rv: rv, ref: Free }),
        devCounter: rv,
        cleared: true,
      },
    }
  }

  action sweepDone(c: str): bool = {
    val cl = s.clusters.get(c)
    all {
      active(cl),
      cl.sweepPhase == 2,
      s' = { ...s, clusters: s.clusters.set(c, { ...cl, sweepPhase: 0 }) },
    }
  }

  // ---- Bind, spec 5.3 and 6.3 ----

  pure def refClaim(ref: OptRef): str = match ref {
    | Held(r) => r.claim
    | Free => ""
  }

  // Set the device ref and give the write a new rv.
  def writeRef(d: str, ref: OptRef): State = {
    val rv = s.devCounter + 1
    { ...s, devices: s.devices.set(d, { rv: rv, ref: ref }), devCounter: rv }
  }

  // Put a bind in flight. The rv comes from a consistent read. The
  // controller adopts first, so no device may name the claim with the
  // current member UID.
  action bindSend(k: str, d: str): bool = {
    val c = CFG.owner.get(k)
    val cl = s.clusters.get(c)
    val cm = s.claims.get(k)
    val dv = s.devices.get(d)
    all {
      localLive(c),
      not(cl.draining),
      cm.exists,
      not(cm.deleting),
      cm.phase == "Pending",
      CFG.devices.forall(e => not(namesWith(s.devices.get(e).ref, k, cl.uid))),
      isFree(dv.ref),
      s.inflight.size() < 3,
      s' = {
        ...s,
        inflight: s.inflight.union(Set({
          cluster: c, claim: k, dev: d, rv: dv.rv, muid: cl.uid,
        })),
      },
    }
  }

  // The bind lands at any later time. The server applies it on the rv
  // condition and rejects a change from one holder to another.
  action bindLand(msg: BindMsg): bool = {
    val dv = s.devices.get(msg.dev)
    val ref = Held({ member: msg.cluster, muid: msg.muid, claim: msg.claim })
    val ok = CFG.unconditionalBind
      or (dv.rv == msg.rv and (isFree(dv.ref) or dv.ref == ref))
    val rest = s.inflight.exclude(Set(msg))
    all {
      s.inflight.contains(msg),
      if (ok)
        s' = { ...writeRef(msg.dev, ref), inflight: rest }
      else
        s' = { ...s, inflight: rest },
    }
  }

  action landSome = all {
    s.inflight.size() > 0,
    nondet msg = s.inflight.oneOf()
    bindLand(msg),
  }

  // Adopt a device that already names the claim with the current member
  // UID. The adoptAnyUid flag drops the UID check.
  action adopt(k: str, d: str): bool = {
    val c = CFG.owner.get(k)
    val cl = s.clusters.get(c)
    val cm = s.claims.get(k)
    val ref = s.devices.get(d).ref
    all {
      active(cl),
      cl.uid != 0,
      not(cl.draining),
      cm.exists,
      not(cm.deleting),
      cm.phase == "Pending",
      if (CFG.adoptAnyUid) names(ref, k) else namesWith(ref, k, cl.uid),
      s' = {
        ...s,
        claims: s.claims.set(k, { ...cm, phase: "Bound", dev: d, muid: cl.uid }),
      },
    }
  }

  // Release a second device that names a claim. Keep the claim's device.
  action releaseDuplicate(k: str, d: str): bool = {
    val cl = s.clusters.get(CFG.owner.get(k))
    val cm = s.claims.get(k)
    all {
      active(cl),
      cm.phase == "Bound" or cm.phase == "Suspended",
      d != cm.dev,
      names(s.devices.get(d).ref, k),
      s' = writeRef(d, Free),
    }
  }

  // Clear a ref that names this member and a claim that no longer exists.
  // A bind can land after the claim's release removed the finalizer.
  action clearOrphan(c: str, d: str): bool = {
    val cl = s.clusters.get(c)
    val ref = s.devices.get(d).ref
    all {
      active(cl),
      refMember(ref) == c,
      not(s.claims.get(refClaim(ref)).exists),
      s' = writeRef(d, Free),
    }
  }

  // ---- Claim phases, delete, and release, spec 6.4 and 6.5 ----

  action suspend(k: str): bool = {
    val c = CFG.owner.get(k)
    val cl = s.clusters.get(c)
    val cm = s.claims.get(k)
    all {
      active(cl),
      cm.phase == "Bound",
      not(localLive(c)) or cl.draining,
      s' = { ...s, claims: s.claims.set(k, { ...cm, phase: "Suspended" }) },
    }
  }

  action resume(k: str): bool = {
    val c = CFG.owner.get(k)
    val cl = s.clusters.get(c)
    val cm = s.claims.get(k)
    all {
      localLive(c),
      not(cl.draining),
      cm.exists,
      not(cm.deleting),
      cm.phase == "Suspended",
      cm.muid == cl.uid,
      namesWith(s.devices.get(cm.dev).ref, k, cl.uid),
      s' = { ...s, claims: s.claims.set(k, { ...cm, phase: "Bound" }) },
    }
  }

  // A user deletes the claim. The finalizer keeps it until release.
  action deleteClaim(k: str): bool = {
    val cl = s.clusters.get(CFG.owner.get(k))
    val cm = s.claims.get(k)
    all {
      active(cl),
      cm.exists,
      not(cm.deleting),
      s' = { ...s, claims: s.claims.set(k, { ...cm, deleting: true }) },
    }
  }

  // Clear each device that names the deleted claim, with any member UID.
  action release(k: str, d: str): bool = {
    val cl = s.clusters.get(CFG.owner.get(k))
    val cm = s.claims.get(k)
    all {
      active(cl),
      cm.exists,
      cm.deleting,
      names(s.devices.get(d).ref, k),
      s' = writeRef(d, Free),
    }
  }

  action removeFinalizer(k: str): bool = {
    val cl = s.clusters.get(CFG.owner.get(k))
    val cm = s.claims.get(k)
    all {
      active(cl),
      cm.exists,
      cm.deleting,
      CFG.devices.forall(d => not(names(s.devices.get(d).ref, k))),
      s' = { ...s, claims: s.claims.set(k, { ...cm, exists: false }) },
    }
  }

  // ---- Graceful leave, spec 7.6 ----

  action startDrain(c: str): bool = {
    val cl = s.clusters.get(c)
    all {
      active(cl),
      cl.uid != 0,
      not(cl.draining),
      s' = { ...s, clusters: s.clusters.set(c, { ...cl, draining: true }) },
    }
  }

  // A draining member releases a device only after its claim is not Bound.
  action drainRelease(k: str, d: str): bool = {
    val cl = s.clusters.get(CFG.owner.get(k))
    val cm = s.claims.get(k)
    all {
      active(cl),
      cl.draining,
      cm.phase != "Bound",
      names(s.devices.get(d).ref, k),
      s' = writeRef(d, Free),
    }
  }

  action finishLeave(c: str): bool = {
    val cl = s.clusters.get(c)
    val mo = s.members.get(c)
    val rv = s.memberCounter + 1
    all {
      active(cl),
      cl.draining,
      cl.uid != 0,
      CFG.devices.forall(d => not(refMember(s.devices.get(d).ref) == c
        and refMuid(s.devices.get(d).ref) == cl.uid)),
      s' = {
        ...s,
        members: s.members.set(c, { ...mo, exists: false, rv: rv }),
        memberCounter: rv,
        clusters: s.clusters.set(c, { ...cl, left: true, uid: 0 }),
        someLeft: true,
      },
    }
  }

  action step = any {
    tick,
    landSome,
    nondet c = CFG.clusters.oneOf()
    nondet m = CFG.clusters.oneOf()
    nondet d = CFG.devices.oneOf()
    nondet k = CFG.claims.oneOf()
    any {
      bindSend(k, d),
      adopt(k, d),
      releaseDuplicate(k, d),
      clearOrphan(c, d),
      suspend(k),
      resume(k),
      deleteClaim(k),
      release(k, d),
      removeFinalizer(k),
      startDrain(c),
      drainRelease(k, d),
      finishLeave(c),
      join(c),
      renewSend(c),
      renewLand(c),
      crash(c),
      recover(c),
      observe(c, m),
      sweepDelete(c, m),
      sweepListDevices(c),
      sweepListMembers(c),
      sweepClear(c, d),
      sweepDone(c),
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
