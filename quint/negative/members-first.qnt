// The sweeper lists members before devices. A member can join and bind
// between the two lists, and the sweeper then clears a valid ref.
// Spec 8.3.
module members_first {
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
    membersFirst: true,
    adoptAnyUid: false,
  }).* from "../solas"
}
