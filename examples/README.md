# Examples

## hello-kernel

A minimal, **offline, zero-key** demo that runs the kernel's core loop in ~5 seconds:

```sh
go run ./examples/hello-kernel
```

It exercises the three foundational pieces and prints their output:

1. **Event bus** — a `kernel.started` doorbell is broadcast to subscribers.
2. **Memory** — two memories are encoded, then one is recalled using the Ebbinghaus
   forgetting-curve retention score.
3. **Emotion** — a five-dimensional state (valence, arousal, intimacy, fatigue,
   curiosity) is updated by the event.

No network, no API key, no model weights are needed — it is a self-contained smoke test
that the core packages wire together correctly.
