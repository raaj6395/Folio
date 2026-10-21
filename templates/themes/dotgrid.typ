#import "clean.typ": theme as base, dark as base-dark
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

#let dark = base-dark + (
  paper-ink: rgb("#3a4359"),
  callout: (
    note: (accent: rgb("#6ea8fe"), fill: rgb("#1a2540ee"), label: "Note"),
    tip: (accent: rgb("#5fd38d"), fill: rgb("#15301fee"), label: "Tip"),
    warning: (accent: rgb("#f5b14c"), fill: rgb("#33260fee"), label: "Warning"),
    definition: (accent: rgb("#b794f6"), fill: rgb("#261d3dee"), label: "Definition"),
    key_point: (accent: rgb("#f47c7c"), fill: rgb("#361a1dee"), label: "Key point"),
  ),
)
