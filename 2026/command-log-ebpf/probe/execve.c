// SPDX-License-Identifier: GPL-2.0
#include "headers/vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>

#define TASK_COMM_LEN    64
#define MAX_FILENAME_LEN 256
#define ARGSIZE          128
#define TOTAL_MAX_ARGS   3

// event is the structure sent from kernel to userspace for each execve call.
struct event {
	__u32 pid;
	__u32 ppid;
	__u32 uid;
	__u32 args_count;
	__u32 args_size;
	__u8  comm[TASK_COMM_LEN];
	__u8  parent_comm[TASK_COMM_LEN];
	__u8  filename[MAX_FILENAME_LEN];
	__u8  args[TOTAL_MAX_ARGS * ARGSIZE];
};

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 256 * 1024);
} events SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_execve")
int tracepoint_sys_enter_execve(struct trace_event_raw_sys_enter *ctx)
{
	struct event *e;
	const char *filename;
	const char *const *argv;
	const char *argp;
	int ret;

	e = bpf_ringbuf_reserve(&events, sizeof(struct event), 0);
	if (!e)
		return 0;

	/* Gather task metadata. */
	__u64 id = bpf_get_current_pid_tgid();
	e->pid = id >> 32;

	struct task_struct *task = (struct task_struct *)bpf_get_current_task();
	e->ppid = BPF_CORE_READ(task, real_parent, tgid);
	e->uid  = (__u32)bpf_get_current_uid_gid();

	bpf_get_current_comm(&e->comm, sizeof(e->comm));

	/* Read the parent process comm for shell filtering. */
	BPF_CORE_READ_STR_INTO(&e->parent_comm, task, real_parent, comm);

	/* Read the executable filename from tracepoint args[0]. */
	filename = (const char *)ctx->args[0];
	bpf_probe_read_user_str(&e->filename, sizeof(e->filename), filename);

	/* Read argv entries (skip argv[0] which duplicates filename). */
	argv = (const char *const *)ctx->args[1];

	e->args_count = 0;
	e->args_size  = 0;

	#pragma unroll
	for (int i = 1; i < TOTAL_MAX_ARGS; i++) {
		argp = NULL;
		ret = bpf_probe_read_user(&argp, sizeof(argp), &argv[i]);
		if (ret < 0 || !argp)
			break;

		/* Ensure there is room for at least one more argument. */
		__u32 off = e->args_size;
		if (off > sizeof(e->args) - ARGSIZE)
			break;

		ret = bpf_probe_read_user_str(
			&e->args[off & (sizeof(e->args) - 1)],
			ARGSIZE,
			argp
		);
		if (ret <= 0)
			break;

		e->args_count++;
		e->args_size = off + ret;
	}

	bpf_ringbuf_submit(e, 0);
	return 0;
}

// Force BTF emission of struct event for bpf2go.
const struct event *unused_event __attribute__((unused));

char LICENSE[] SEC("license") = "GPL";
