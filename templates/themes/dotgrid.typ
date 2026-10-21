#import "clean.typ": theme as base
#let theme = base + (
  paper-ink: rgb("#9aa3b2"),
  paper-step: 5mm,
  callout-style: "box",
  callout-radius: 5pt,
  callout: (
    note: (accent: rgb("#2563eb"), fill: rgb("#eff4ffee"), label: "Note"),
    tip: (accent: rgb("#16a34a"), fill: rgb("#ecfdf3ee"), label: "Tip"),
    warning: (accent: rgb("#d97706"), fill: rgb("#fff7e8ee"), label: "Warning"),
    definition: (accent: rgb("#7c3aed"), fill: rgb("#f5f0ffee"), label: "Definition"),
    key_point: (accent: rgb("#dc2626"), fill: rgb("#fef1f1ee"), label: "Key point"),
  ),
)
