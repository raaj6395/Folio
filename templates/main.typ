#let doc = json("doc.json")
#import ("themes/" + doc.theme + ".typ"): theme, dark
#let t = if doc.at("mode", default: "light") == "dark" { theme + dark } else { theme }
#let density = doc.at("density", default: "legacy")
#let g = (
  legacy: (h1: 1.6em, h2: 1.3em, h3: 1.3em, box: 1em, callout: 1.1em, block: auto),
  compact: (h1: 1.4em, h2: 1.1em, h3: 1em, box: 0.8em, callout: 0.8em, block: 0.9em),
  comfortable: (h1: 3em, h2: 2.6em, h3: 1.8em, box: 1.6em, callout: 1.6em, block: 1.3em),
  airy: (h1: 3.6em, h2: 3.2em, h3: 2.2em, box: 2.1em, callout: 2.1em, block: 1.6em),
).at(density, default: (h1: 1.6em, h2: 1.3em, h3: 1.3em, box: 1em, callout: 1.1em, block: auto))

#let paper-background = {
  let ink = t.paper-ink
  let step = t.paper-step
  let body = if doc.paper == "ruled" {
    place(top + left, dy: 20mm, rect(width: 100%, height: 100% - 34mm, stroke: none,
      fill: tiling(size: (step, step), place(bottom, line(length: step, stroke: 0.6pt + ink)))))
  } else if doc.paper == "dotted" {
    rect(width: 100%, height: 100%, stroke: none,
      fill: tiling(size: (step, step), place(dx: step / 2, dy: step / 2, circle(radius: 0.45pt, fill: ink, stroke: none))))
  } else if doc.paper == "grid" {
    rect(width: 100%, height: 100%, stroke: none,
      fill: tiling(size: (step, step), {
        place(top, line(length: step, stroke: 0.3pt + ink))
        place(left, line(angle: 90deg, length: step, stroke: 0.3pt + ink))
      }))
  }
  body
  if t.margin-line != none {
    place(top + left, dx: 14mm, line(angle: 90deg, length: 100%, stroke: 0.8pt + t.margin-line))
  }
}

#set document(title: doc.title)
#set page(
  paper: "a4",
  margin: (x: 18mm, y: 20mm),
  fill: t.page-bg,
  background: paper-background,
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
#set raw(theme: t.code-theme)
#show heading: set text(font: t.heading-font, fill: t.heading)
#show heading.where(level: 1): set text(size: t.heading-sizes.at(0), weight: "bold")
#show heading.where(level: 2): set text(size: t.heading-sizes.at(1), weight: "bold")
#show heading.where(level: 3): set text(size: t.heading-sizes.at(2), weight: "semibold")
#show link: set text(fill: t.accent)
#show: body => if g.block == auto { body } else {
  set block(spacing: g.block)
  body
}

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

#let nosplit(body, above: auto, below: auto, never: false, limit: 0.4) = {
  let inner = layout(size => {
    let h = measure(block(width: size.width, body)).height
    block(width: 100%, breakable: not never and h > limit * content-height, above: above, below: below, body)
  })
  if density == "legacy" { inner } else { block(width: 100%, above: above, below: below, inner) }
}

#let caption(xs, at: center) = if xs.len() > 0 {
  align(at, text(size: 0.85em, fill: t.muted, spans(xs)))
}

#let render(b) = {
  let ty = b.type
  if ty == "heading" {
    let above = (g.h1, g.h2, g.h3).at(b.level - 1)
    block(sticky: true, above: above, below: 0.7em, {
      mark(b.id)
      heading(level: b.level, spans(b.text))
      endmark(b.id)
    })
  } else if ty == "paragraph" {
    block(width: 100%, { mark(b.id); spans(b.text); endmark(b.id) })
  } else if ty == "bullets" or ty == "numbered" {
    nosplit(limit: 0.25, block(width: 100%, { mark(b.id); items(b.items, ty == "numbered"); endmark(b.id) }))
  } else if ty == "callout" {
    let cv = t.callout.at(b.variant)
    nosplit(above: g.callout, below: g.callout, block(
      width: 100%,
      fill: cv.fill,
      stroke: if t.callout-style == "box" { 1.1pt + cv.accent } else { (left: 3pt + cv.accent) },
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
  } else if ty == "table" {
    let n = b.columns.len()
    let aligns = b.at("align", default: ("left",) * n).map(a => (left: left, center: center, right: right).at(a))
    nosplit(above: g.box, below: g.box, block(width: 100%, {
      mark(b.id)
      table(
        columns: (auto,) * n,
        align: (x, _) => aligns.at(x) + horizon,
        stroke: 0.5pt + t.rule,
        inset: (x: 6pt, y: 5pt),
        fill: (_, y) => if y == 0 { t.table-header },
        table.header(..b.columns.map(c => text(weight: "bold", spans(c)))),
        ..b.rows.map(r => r.map(c => spans(c))).flatten(),
      )
      caption(b.caption, at: left)
      endmark(b.id)
    }))
  } else if ty == "qa" {
    counter("qa").step()
    nosplit(above: g.box, below: g.box, block(width: 100%, stroke: 0.7pt + t.qa-accent, radius: t.callout-radius, inset: (x: 10pt, y: 8pt), {
      mark(b.id)
      context [#text(weight: "bold", fill: t.qa-accent)[Q#counter("qa").display().] ]
      spans(b.question)
      if b.hide_answer {
        block(above: 0.7em, text(size: 0.85em, fill: t.muted, style: "italic")[Answer: upside down at the bottom of this page.])
      } else {
        block(above: 0.7em, width: 100%, stroke: (top: 0.5pt + t.rule), inset: (top: 6pt), {
          text(weight: "bold", fill: t.qa-accent)[A. ]
          spans(b.answer)
        })
      }
      endmark(b.id)
    }))
    if b.hide_answer {
      place(bottom, float: true, clearance: 8pt, rotate(180deg, block(width: 100%, text(size: 0.8em, fill: t.muted, {
        context [*A#counter("qa").display().* ]
        spans(b.answer)
      }))))
    }
  } else if ty == "fill_in" {
    nosplit(above: g.box, below: g.box, block(width: 100%, {
      mark(b.id)
      text(weight: "semibold", spans(b.prompt))
      for _ in range(b.lines) {
        v(7.5mm, weak: false)
        line(length: 100%, stroke: 0.5pt + t.fill-line)
      }
      v(2mm)
      endmark(b.id)
    }))
  } else if ty == "spacer" {
    let h = (small: 4mm, medium: 10mm, large: 20mm).at(b.size)
    block(width: 100%, { mark(b.id); block(width: 100%, height: h); endmark(b.id) })
  } else if ty == "diagram" {
    nosplit(above: g.box, below: g.box, never: true, block(width: 100%, {
      mark(b.id)
      layout(size => align(center, image(b.svg, width: calc.min(b.width_mm * 1mm, size.width))))
      caption(b.caption)
      endmark(b.id)
    }))
  } else if ty == "two_column" {
    nosplit(above: g.box, below: g.box, limit: 0.9, block(width: 100%, {
      mark(b.id)
      grid(columns: (1fr, 1fr), column-gutter: 7mm,
        { for k in b.left { render(k) } },
        { for k in b.right { render(k) } })
      endmark(b.id)
    }))
  } else if ty == "divider" {
    block(width: 100%, above: g.box, below: g.box, { mark(b.id); line(length: 100%, stroke: 0.6pt + t.rule); endmark(b.id) })
  } else if ty == "page_break" {
    mark(b.id)
    endmark(b.id)
    pagebreak(weak: true)
  }
}

#for b in doc.blocks { render(b) }
#mark("_end")
