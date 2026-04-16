The cpus command provides a simple expression language to select a set of CPUs
and run a command limited to those CPUs.

For detailed command documentation, see
[pkg.go.dev](https://pkg.go.dev/github.com/aclements/cpus/cmd/cpus)

# Examples

To see the topology of your system, run:

  ./cpus --format table

On an 88 CPU machine with two hyperthreads per core and multiple sockets and
NUMA nodes, this may print something like

  node socket core thread processor
  0    0      0    0      0
  0    0      0    1      44
  0    0      1    0      1
  0    0      1    1      45
  ...
  1    1      43   0      43
  1    1      43   1      87

If we want to run a command without hyperthread interference, we can `cpus` in
"taskset" mode with a filter:

  ./cpus thread==0 -- ls

If we want a command to have 8 CPUs, preferring to keep it on a single NUMA
node, we can use a sorter with a limit.

  ./cpus -limit 8 node -- ls

# Benchmark sweeps

We can use "sweep" syntax to run scalability benchmarks across a sequence of CPU
masks. A basic sweep looks like

  ./cpus -limit 1.. -- go test . -run ^$ -bench .

This will run a Go benchmark with 1 CPU, then 2 CPUs, etc.

We can combine this with filters and sorters to achieve complex effects. For
example, to minimize hyperthread interactions, we can limit the sweep to only
run on the first hyperthread of each core with

  ./cpus -limit 1.. thread==0 -- go test . -run ^$ -bench .

We can use a sort to make sure the benchmark fills up resources in a predictable
order. For example, this uses the "round robin" order to fill up each socket and
NUMA node before spilling on the next, while still limiting the command to one
hyperthread

  ./cpus -limit 1.. thread==0 rr -- go test . -run ^$ -bench .

Sweeps can also be used to print CPU sets that can be passed to other commands.
The following command uses a shell loop with `taskset` to implement CPU masks

  for mask in $(cpus -limit 1.. rr); do taskset -c $mask ls; done
