// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package cmdline

// metacharacters are the shell operators this deliberately does not implement.
//
// They are rejected rather than passed through as ordinary text. Silently
// treating "battery > out.txt" as a command with two extra arguments would let
// somebody believe a redirect had happened, and the file they went looking for
// would never appear.
const metacharacters = "|&;<>()$`"
