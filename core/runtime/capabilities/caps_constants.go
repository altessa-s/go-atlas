// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package capabilities

// Canonical Linux capability bits. Values mirror the kernel ABI and
// are stable across kernel releases (new caps are appended; existing
// numbers are never reused). Declared here — not via an import of
// [golang.org/x/sys/unix] — so that the typed constants are visible
// on every platform, letting cross-platform code write
// capabilities.CAP_NET_BIND_SERVICE without build tags.
//
// Source: include/uapi/linux/capability.h in the Linux kernel tree.
const (
	CAP_CHOWN              Cap = 0
	CAP_DAC_OVERRIDE       Cap = 1
	CAP_DAC_READ_SEARCH    Cap = 2
	CAP_FOWNER             Cap = 3
	CAP_FSETID             Cap = 4
	CAP_KILL               Cap = 5
	CAP_SETGID             Cap = 6
	CAP_SETUID             Cap = 7
	CAP_SETPCAP            Cap = 8
	CAP_LINUX_IMMUTABLE    Cap = 9
	CAP_NET_BIND_SERVICE   Cap = 10
	CAP_NET_BROADCAST      Cap = 11
	CAP_NET_ADMIN          Cap = 12
	CAP_NET_RAW            Cap = 13
	CAP_IPC_LOCK           Cap = 14
	CAP_IPC_OWNER          Cap = 15
	CAP_SYS_MODULE         Cap = 16
	CAP_SYS_RAWIO          Cap = 17
	CAP_SYS_CHROOT         Cap = 18
	CAP_SYS_PTRACE         Cap = 19
	CAP_SYS_PACCT          Cap = 20
	CAP_SYS_ADMIN          Cap = 21
	CAP_SYS_BOOT           Cap = 22
	CAP_SYS_NICE           Cap = 23
	CAP_SYS_RESOURCE       Cap = 24
	CAP_SYS_TIME           Cap = 25
	CAP_SYS_TTY_CONFIG     Cap = 26
	CAP_MKNOD              Cap = 27
	CAP_LEASE              Cap = 28
	CAP_AUDIT_WRITE        Cap = 29
	CAP_AUDIT_CONTROL      Cap = 30
	CAP_SETFCAP            Cap = 31
	CAP_MAC_OVERRIDE       Cap = 32
	CAP_MAC_ADMIN          Cap = 33
	CAP_SYSLOG             Cap = 34
	CAP_WAKE_ALARM         Cap = 35
	CAP_BLOCK_SUSPEND      Cap = 36
	CAP_AUDIT_READ         Cap = 37
	CAP_PERFMON            Cap = 38
	CAP_BPF                Cap = 39
	CAP_CHECKPOINT_RESTORE Cap = 40

	// capLastCap is the highest Cap value this package knows about,
	// mirroring Linux's CAP_LAST_CAP. Package-private because a
	// future kernel may add more caps and the exported API should
	// not require a package rebuild to recognize them.
	capLastCap = CAP_CHECKPOINT_RESTORE
)

// capToName maps each known Cap to its canonical "CAP_*" string.
// Populated at init from the inverse of the constant block above.
var capToName = map[Cap]string{
	CAP_CHOWN:              "CAP_CHOWN",
	CAP_DAC_OVERRIDE:       "CAP_DAC_OVERRIDE",
	CAP_DAC_READ_SEARCH:    "CAP_DAC_READ_SEARCH",
	CAP_FOWNER:             "CAP_FOWNER",
	CAP_FSETID:             "CAP_FSETID",
	CAP_KILL:               "CAP_KILL",
	CAP_SETGID:             "CAP_SETGID",
	CAP_SETUID:             "CAP_SETUID",
	CAP_SETPCAP:            "CAP_SETPCAP",
	CAP_LINUX_IMMUTABLE:    "CAP_LINUX_IMMUTABLE",
	CAP_NET_BIND_SERVICE:   "CAP_NET_BIND_SERVICE",
	CAP_NET_BROADCAST:      "CAP_NET_BROADCAST",
	CAP_NET_ADMIN:          "CAP_NET_ADMIN",
	CAP_NET_RAW:            "CAP_NET_RAW",
	CAP_IPC_LOCK:           "CAP_IPC_LOCK",
	CAP_IPC_OWNER:          "CAP_IPC_OWNER",
	CAP_SYS_MODULE:         "CAP_SYS_MODULE",
	CAP_SYS_RAWIO:          "CAP_SYS_RAWIO",
	CAP_SYS_CHROOT:         "CAP_SYS_CHROOT",
	CAP_SYS_PTRACE:         "CAP_SYS_PTRACE",
	CAP_SYS_PACCT:          "CAP_SYS_PACCT",
	CAP_SYS_ADMIN:          "CAP_SYS_ADMIN",
	CAP_SYS_BOOT:           "CAP_SYS_BOOT",
	CAP_SYS_NICE:           "CAP_SYS_NICE",
	CAP_SYS_RESOURCE:       "CAP_SYS_RESOURCE",
	CAP_SYS_TIME:           "CAP_SYS_TIME",
	CAP_SYS_TTY_CONFIG:     "CAP_SYS_TTY_CONFIG",
	CAP_MKNOD:              "CAP_MKNOD",
	CAP_LEASE:              "CAP_LEASE",
	CAP_AUDIT_WRITE:        "CAP_AUDIT_WRITE",
	CAP_AUDIT_CONTROL:      "CAP_AUDIT_CONTROL",
	CAP_SETFCAP:            "CAP_SETFCAP",
	CAP_MAC_OVERRIDE:       "CAP_MAC_OVERRIDE",
	CAP_MAC_ADMIN:          "CAP_MAC_ADMIN",
	CAP_SYSLOG:             "CAP_SYSLOG",
	CAP_WAKE_ALARM:         "CAP_WAKE_ALARM",
	CAP_BLOCK_SUSPEND:      "CAP_BLOCK_SUSPEND",
	CAP_AUDIT_READ:         "CAP_AUDIT_READ",
	CAP_PERFMON:            "CAP_PERFMON",
	CAP_BPF:                "CAP_BPF",
	CAP_CHECKPOINT_RESTORE: "CAP_CHECKPOINT_RESTORE",
}

// nameToCap is the reverse of capToName, populated at init. Lookups
// use upper-cased keys so ParseName can be case-insensitive cheaply.
var nameToCap = func() map[string]Cap {
	m := make(map[string]Cap, len(capToName))
	for c, name := range capToName {
		m[name] = c
	}
	return m
}()
