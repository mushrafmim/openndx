// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

// Package authn validates JWT access tokens and exposes the caller as a single
// principal. See README.md.
//
// This is a temporary, unmodified copy of github.com/OpenNSW/core/authn from
// https://github.com/mushrafmim/nsw-core, branch refactor/291-single-principal,
// commit 1e6f75d. OpenNSW still depends on the older authn, so this version is
// not released upstream yet. Do not add OpenNDX-specific changes here; chain
// application middleware after authn instead. Once OpenNSW/core releases this
// version, delete this package and import the upstream module.
package authn
