Unknown formatting tags drop the tag character:
use `code` here
ab`Xcd
trailing `

Heading alignment tokens apply to the heading:
>>`cCentered heading
>>`rRight heading
>>Plain heading

Anchors may be Unicode, and fields accept Python-style integer widths:
`:café
See [the café anchor`#café]
`<-5|negative`>
`< 24|spaced`>
`<1_0|underscored`>

Literal blocks only unescape a whole-line escape:
`=
literal \`= inside
\`=
`=

Malformed commands drop their command character:
bad `X color
empty `: anchor
