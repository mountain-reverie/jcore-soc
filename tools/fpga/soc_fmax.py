#!/usr/bin/env python3
"""Read Fmax and the binding critical path out of a ULX3S metrics.json.

Companion to emit_metrics.py, which produces the file. Split out so the numbers
can be unit-tested without running a multi-minute place-and-route.

Why the critical path matters as much as Fmax: it names the source register and
final sink of the binding path, with the logic/routing split. If that endpoint
moves at some commit, it says *where* the cost went, turning "commit X is bad"
into "commit X did this specific thing".
"""
import json
import statistics


def fmax_from_metrics(doc):
    """MHz value of the doc's single '<...>/Fmax' metric.

    emit_metrics names it '<board> [<variant>]/Fmax', so match on the suffix
    rather than a fixed string. Ambiguity is an error, not a coin flip: two
    Fmax series would mean the caller cannot tell which variant it measured.
    """
    hits = [m for m in doc.get("metrics", []) if str(m.get("name", "")).endswith("/Fmax")]
    if not hits:
        raise ValueError("no '/Fmax' metric in doc")
    if len(hits) > 1:
        raise ValueError("ambiguous: %d '/Fmax' metrics: %s"
                         % (len(hits), [m.get("name") for m in hits]))
    return float(hits[0]["value"])


def critical_path_from_metrics(doc):
    """The critical_path object, or None.

    emit_metrics only sets this key when it managed to parse nextpnr's report,
    so absence is normal and must not be treated as an error.
    """
    return doc.get("critical_path")


def summarize(values):
    """{'min','median','max','n'} for a non-empty list of measurements."""
    if not values:
        raise ValueError("no measurements to summarize")
    return {
        "min": min(values),
        "median": statistics.median(values),
        "max": max(values),
        "n": len(values),
    }


def noise_band(values):
    """Spread of a measurement set, in MHz."""
    return max(values) - min(values)


def main(argv=None):
    import argparse
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument("--metrics", required=True, help="path to metrics.json")
    p.add_argument("--json", action="store_true", help="emit JSON")
    a = p.parse_args(argv)

    with open(a.metrics) as f:
        doc = json.load(f)
    out = {"fmax": fmax_from_metrics(doc),
           "critical_path": critical_path_from_metrics(doc)}
    if a.json:
        print(json.dumps(out))
    else:
        cp = out["critical_path"]
        print("fmax %.2f MHz" % out["fmax"])
        if cp:
            print("critical path: %s -> %s (%s ns logic, %s ns routing)"
                  % (cp.get("source"), cp.get("sink"),
                     cp.get("logic_ns"), cp.get("routing_ns")))
        else:
            print("critical path: not reported")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
