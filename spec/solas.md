# Solas specification

## 1. Introduction

### 1.1. Scope

This document specifies solas. Solas is a Kubernetes extension API
server that stores global resources in DynamoDB. Several member clusters
share one table and so share one view of the global resources. The
document also specifies the controller that binds devices to claims.

### 1.2. Terms

- **Member cluster**: a Kubernetes cluster that runs the solas API server
  and the solas controller.
- **Table**: the one DynamoDB table that all member clusters share.
- **Resource**: a group and resource name, for example
  `solas.dev/devices`.
- **Write**: a create, update, or delete of one object.
- **Resource version**: the `metadata.resourceVersion` of an object, or
  the version of a list or a watch.
- **Holder**: the claim that a `Device.status.claimRef` names.
- **Observer**: a member cluster that measures the lease of another
  member.

### 1.3. Conventions

The key words MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are to be
interpreted as described in RFC 2119 and RFC 8174 when, and only when,
they appear in all capitals.

Each normative sentence holds one requirement. Code cites the sentence
with a Duvet annotation.

## 2. Storage

## 3. Resource versions

## 4. Watch

## 5. Device

## 6. Claim

## 7. Membership

## 8. Reclaim
