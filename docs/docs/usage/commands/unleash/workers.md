# Workers

Gremlins works in parallel mode. It uses some sensible defaults, but it may be necessary to tweak them in your specific
use case. Finding the correct settings mandates a little trial and error, and we are still learning how to get the most
of it.

The first setting you should be aware of is the number of _workers_ (`--workers`). By default, Gremlins uses the number
of available CPU cores. This value is correct most of the time, but if you notice an excessive number of mutations going
into `TIMED OUT`, you may try to decrease this value.

If you decrease this value, you may also try to increase the number of CPU cores available to each test
run (`--test-cpu`). This is equivalent to the `-cpu` flag of the Go test tool, but for each mutation test. Gremlins
doesn't enforce this by default.

A rule of thumb may be setting it so that the sum of _workers_ and _test CPU_ is equal to the total number of cores of
of the machine.

The symptom of a run excessively stressed is the number of mutants going into `TIMED OUT`. You should tweak the two
values above until your runs stabilize on a low and constant number of `TIMED OUT` mutants. To understand what could be
your correct value, you can run Gremlins with a single worker and see the results.

## Timeout coefficient

Another setting you may want to tweak is the _timeout coefficient_. This is the multiplier used to increase the
estimated time it takes to do a run of the tests. The default value should be ok, but if you see too much tests timing
out, then you may try to play a little with this value. Don't increase it too much though, or the run might become
excessively slow. At the moment, it defaults to 3.

If your test suite takes a lot of time to run, you may want to tweak this setting to _decrease_ the coefficient. We are
thinking of a dynamic way to set this, but it is not clear yet the correct algorithm to use.

## Integration mode

_Integration mode_ is quite heavy on the CPU in parallel mode. For this reason, Gremlins halves the values for workers
and _test CPU_ if it is running in _integration mode_. So, if you set, for example, 4 workers, it will run effectively
with 2. And same goes for _test CPU_.
