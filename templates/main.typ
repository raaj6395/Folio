#let doc = json("doc.json")
#import ("themes/" + doc.theme + ".typ"): theme
#let t = theme

#set document(title: doc.title)
#set page(
  paper: "a4",
  margin: (x: 18mm, y: 20mm),
  header: context {
    set text(size: 8pt, fill: t.muted)
    doc.title
    h(1fr)
  },
  footer: context {
    set text(size: 8pt, fill: t.muted)
    h(1fr)
    counter(page).display("1")
    h(1fr)
  },
)
#set text(font: t.body-font, size: t.body-size, fill: t.ink, lang: "en")
#set par(leading: t.leading, justify: false)
#show raw: set text(font: t.mono-font, size: 0.9em)
#show heading: set text(font: t.heading-font, fill: t.heading)
#show heading.where(level: 1): set text(size: t.heading-sizes.at(0), weight: "bold")
#show heading.where(level: 2): set text(size: t.heading-sizes.at(1), weight: "bold")
#show heading.where(level: 3): set text(size: t.heading-sizes.at(2), weight: "semibold")
#show link: set text(fill: t.accent)

#let spans(xs) = for s in xs {
  let c = if s.at("c", default: false) {
    box(fill: t.code-bg, inset: (x: 2.5pt), outset: (y: 2.5pt), radius: 2pt, raw(s.t))
  } else { text(s.t) }
  if s.at("b", default: false) { c = strong(c) }
  if s.at("i", default: false) { c = emph(c) }
  if s.at("h", default: false) { c = highlight(c, fill: t.highlight) }
  if "l" in s { c = link(s.l, c) }
  c
}

#let items(xs, numbered) = {
  let f = if numbered { enum } else { list }
  f(..xs.map(it => {
    spans(it.text)
    if it.children.len() > 0 { items(it.children, numbered) }
  }))
}

#let mark(id) = [#metadata((id: id, k: "s"))<blockpos>]
#let endmark(id) = [#metadata((id: id, k: "e"))<blockpos>]

#let content-height = 257mm

#let nosplit(body, above: auto, below: auto) = layout(size => {
  let h = measure(block(width: size.width, body)).height
  block(width: 100%, breakable: h > 0.4 * content-height, above: above, below: below, body)
})

#let caption(xs) = if xs.len() > 0 {
  align(center, text(size: 0.85em, fill: t.muted, spans(xs)))
}

#let render(b) = {
  let ty = b.type
  if ty == "heading" {
    let above = if b.level == 1 { 1.6em } else { 1.3em }
    block(sticky: true, above: above, below: 0.7em, {
      mark(b.id)
      heading(level: b.level, spans(b.text))
      endmark(b.id)
    })
  } else if ty == "paragraph" {
    block(width: 100%, { mark(b.id); spans(b.text); endmark(b.id) })
  } else if ty == "bullets" or ty == "numbered" {
    block(width: 100%, { mark(b.id); items(b.items, ty == "numbered"); endmark(b.id) })
  } else if ty == "callout" {
    let cv = t.callout.at(b.variant)
    nosplit(above: 1.1em, below: 1.1em, block(
      width: 100%,
      fill: cv.fill,
      stroke: (left: 3pt + cv.accent),
      radius: t.callout-radius,
      inset: (x: 10pt, y: 8pt),
      {
        set par(spacing: 0.65em)
        mark(b.id)
        let title = if b.title.len() > 0 { spans(b.title) } else { cv.label }
        block(below: 0.55em, text(weight: "bold", fill: cv.accent, title))
        for (i, p) in b.body.enumerate() {
          if i > 0 { parbreak() }
          spans(p)
        }
        endmark(b.id)
      },
    ))
  } else if ty == "code" {
    nosplit(block(width: 100%, {
      mark(b.id)
      block(width: 100%, fill: t.code-bg, radius: 3pt, inset: 8pt,
        raw(b.code, lang: if b.language == "" { none } else { b.language }, block: true))
      caption(b.caption)
      endmark(b.id)
    }))
  } else if ty == "divider" {
    block(width: 100%, above: 1em, below: 1em, { mark(b.id); line(length: 100%, stroke: 0.6pt + t.rule); endmark(b.id) })
  } else if ty == "page_break" {
    mark(b.id)
    endmark(b.id)
    pagebreak(weak: true)
  }
}

#for b in doc.blocks { render(b) }
#mark("_end")
