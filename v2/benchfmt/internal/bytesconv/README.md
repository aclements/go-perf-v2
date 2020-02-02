This is a partial copy of strconv.Parse* from Go 1.13.6, converted to
use []byte (and stripped of the insane extFloat fast-path). It makes
me sad that we have to do this, but see golang.org/issue/2632. We can
eliminate this if golang.org/issue/2205 gets fixed.
