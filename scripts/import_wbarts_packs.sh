#!/usr/bin/env bash
set -euo pipefail

wbarts=${1:-../WBArts}
output=${2:-cards}
sets=${3:-10000,10001}
cards_json="$wbarts/data/cards.json"

if [[ ! -f "$cards_json" ]]; then
  printf 'missing WBArts card data: %s\n' "$cards_json" >&2
  exit 1
fi

IFS=',' read -r -a set_ids <<< "$sets"
for set_id in "${set_ids[@]}"; do
  if [[ ! "$set_id" =~ ^[0-9]+$ ]]; then
    printf 'invalid card set id: %s\n' "$set_id" >&2
    exit 1
  fi
done
mkdir -p "${set_ids[@]/#/$output/}"

jq -c --arg sets "$sets" '($sets | split(",") | map(tonumber)) as $wanted | .[] | select(.card_set_id as $id | $wanted | index($id))' "$cards_json" |
while IFS= read -r card; do
  id=$(jq -r '.card_id' <<<"$card")
  pack=$(jq -r '.card_set_id' <<<"$card")
  path="$output/$pack/$id.wbo"
  [[ -e "$path" ]] && continue

  type=$(jq -er '({"1":"follower","2":"amulet","3":"spell"})[.type | tostring] // error("unknown card type")' <<<"$card")
  trait=$(jq -r '
    {"0":"", "2":"officer", "3":"luminous", "4":"levin", "5":"pixie",
     "6":"departed", "8":"earthsigil", "11":"mysteria", "12":"golem",
     "13":"shikigami", "14":"artifact", "15":"puppetry", "17":"marine",
     "18":"loot", "19":"encroacher", "20":"anathema"} as $traits |
    (.tribe // 0 | tostring) as $id |
    if $traits | has($id) then $traits[$id] else error("unknown tribe id: " + $id) end
  ' <<<"$card")
  cost=$(jq -r '.cost' <<<"$card")
  stats=$(jq -r 'if .type == 1 then "\(.atk)/\(.life)" else empty end' <<<"$card")
  name=$(jq -r '.name_chs // ""' <<<"$card")
  text=$(jq -r '[.skill_texts[]?.text_chs] | join("\\n")' <<<"$card")
  rarity=$(jq -r 'if .rarity == 1 then "bronze" elif .rarity == 2 then "silver" elif .rarity == 3 then "gold" else "legendary" end' <<<"$card")
  class=$(jq -r '(["neutral","forestcraft","swordcraft","runecraft","dragoncraft","abysscraft","havencraft","portalcraft"])[.class] // "neutral"' <<<"$card")

  {
    printf 'wbo 0.1.0;\n\ncard %s {\n    type %s;\n    cost %s;\n' "$id" "$type" "$cost"
    [[ -n "$stats" ]] && printf '    stats %s;\n' "$stats"
    [[ -n "$trait" ]] && printf '    trait %s;\n' "$trait"
    printf '\n    effect {\n        unplayable;\n    }\n\n    meta {\n        pack %s;\n        class %s;\n        rarity %s;\n    }\n\n' "$pack" "$class" "$rarity"
    for locale in chs eng jpn kor cht; do
      key="name_$locale"
      value=$(jq -r --arg key "$key" '.[$key] // ""' <<<"$card" | sed 's/\\/\\\\/g; s/"/\\"/g')
      locale_text=$(jq -r --arg locale "$locale" '[.skill_texts[]?.["text_\($locale)"]] | join("\\n")' <<<"$card" | sed 's/\\/\\\\/g; s/"/\\"/g')
      printf '    locale %s {\n        name "%s";\n        text """\n%s\n""";\n    }\n\n' "$locale" "$value" "$locale_text"
    done
    printf '}\n'
  } > "$path"
done

printf 'Imported missing card definitions from sets %s into %s\n' "$sets" "$output"
