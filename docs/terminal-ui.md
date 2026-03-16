# Terminal UI Guidelines

This document defines how Mem should look and behave in interactive terminals.

The goal is not to make the CLI feel like a small GUI. The goal is to make it read like a well-structured technical document: fast to scan, quiet by default, and explicit when something needs attention.

## Scope

Apply these rules to:
- `mem --help` and command help output
- status and diagnostics output such as `doctor`
- progress and long-running command feedback
- any future interactive terminal surfaces

## Core Stance

Design for plain text first.

A good terminal UI for this repo should be:
- text-first
- stable
- sparse
- semantically colored
- readable in low-fidelity terminals

If a screen works well without color or motion, richer terminals can enhance it. If it only works with color or animation, it is not robust enough.

## Layout Rules

Use a calm layout:
- Prefer one main reading column.
- Keep labels and values aligned.
- Use spacing and indentation before box-drawing characters.
- Keep section order stable across runs.
- Keep status placement consistent so users can scan vertically.

Favor recognition over recall:
- Every screen should answer: where am I, what is happening, what can I do next.
- Put the command or subject first.
- Put actionable guidance near the end, not buried in prose.
- Use tables only when comparison matters.

Keep normal operation visually quiet:
- Routine success should be brief.
- Warnings and errors should be prominent.
- Avoid turning every line into a banner, badge, or callout.

## Color Rules

Use a semantic palette, not decorative color.

Preferred roles:
- body text: terminal default foreground
- secondary metadata: muted or dim text
- accent: one accent color only
- success: green
- warning: yellow or amber
- error: red
- info: blue or cyan

Constraints:
- Never rely on color alone to convey meaning.
- Pair status color with text such as `OK`, `WARN`, or `ERROR`.
- Avoid using red and green as the only differentiator between states.
- Do not dim primary content or critical instructions.
- Avoid highly saturated color combinations that reduce legibility on dark themes.

Implementation defaults:
- Respect `NO_COLOR`.
- Assume 16-color terminals first.
- Let the terminal theme supply the actual foreground/background pair whenever possible.

## Motion Rules

Use motion only when it explains state.

Allowed uses:
- a spinner for short waits with unknown duration
- a progress bar, step count, or percentage for longer work
- limited transitions that preserve continuity between states

Avoid:
- decorative animation
- pulsing, bouncing, or shimmering effects
- moving backgrounds
- rapid screen rewrites that make logs hard to follow

Operational rules:
- Disable or suppress motion in non-interactive contexts such as CI, logs, and pipes.
- Prefer periodic textual updates over high-frequency repainting.
- Preserve stable line positions whenever possible.

## Accessibility Rules

Baseline requirements:
- The UI must remain usable without color.
- Important distinctions need a second cue: wording, position, symbol, or grouping.
- Keep contrast high enough for common dark and light terminal themes.
- Prefer short line lengths when possible.
- Do not encode links or focus states by color alone; use underline, inverse, or explicit labels.

## Mem-Specific Guidance

For this repo, terminal output should generally follow this shape:
- title or command context
- one compact summary line when useful
- clearly separated sections
- aligned rows for commands, flags, or status fields
- short notes that tell the user what to do next

Examples of good defaults:
- `mem --help`: compact sections, aligned commands, no decorative framing
- `mem doctor`: plain status summary, stable field order, explicit remediation text
- long-running ingest or embedding work: progress with counts or phases, not constant spinner noise

Branding should stay restrained:
- If Mem uses an ASCII logo, reserve it for entry surfaces such as top-level help and successful `init`.
- Keep the mark compact enough that command context and next steps still remain the primary content.

## Evidence Base

This guidance is informed by a mix of usability, accessibility, readability, and visualization research rather than a single terminal-specific standard.

Useful references:
- Nielsen, J. (1994). Enhancing the explanatory power of usability heuristics.
- Myers, B. A. (1986). A taxonomy of user interfaces.
- Ware, C. (2012). Information Visualization: Perception for Design.
- Cleveland, W. S., and McGill, R. (1984). Graphical perception.
- Hall, R. H., and Hanna, P. (2004). Text-background color combinations and readability.
- Heer, J., and Robertson, G. (2007). Animated transitions in statistical graphics.
- Robertson, G. G., Fernandez, R., Fisher, D., Lee, B., and Stasko, J. (2008). Animation in trend visualization.
- Szafir, D. A. (2018). Five ways visualizations can mislead.
- W3C. Web Content Accessibility Guidelines (WCAG) 2.2.
- Norman, D. A. (2013). The Design of Everyday Things.

## What Is Not Settled

The evidence is weaker on terminal-specific hex palettes, ideal frame rates, and a single best animation style. For those details, favor conservative terminal conventions:
- plain text first
- semantic ANSI roles
- optional richer rendering only when the environment supports it
