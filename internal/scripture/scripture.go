// Package scripture is the shared verse/quote loader used by both the
// granit TUI's scripture overlay and the web's /scripture page (plus
// the dashboard's "verse of the day" widget).
//
// Source of truth: <vault>/.granit/scriptures.md — one entry per line,
// with optional " — Source" / " – Source" / " - Source" suffix to
// separate the quote text from its citation. The TUI established this
// format; we keep it byte-for-byte compatible so a vault edited in
// either surface stays portable.
package scripture

import (
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Scripture represents a single verse / quote entry. Topics is an
// optional list of theme tags ("hope", "work", "suffering", …) used by
// the topical browser; it's only populated for the built-in catalogue —
// user-edited .granit/scriptures.md entries don't carry topics today.
type Scripture struct {
	Text   string   `json:"text"`             // the verse or quote text
	Source string   `json:"source,omitempty"` // e.g. "Proverbs 3:5-6" or "Marcus Aurelius"
	Topics []string `json:"topics,omitempty"` // theme tags for /scripture topical browse
}

// Load reads scriptures from <vault>/.granit/scriptures.md, returning
// the built-in defaults when the file is missing or empty. Lines starting
// with '#' are treated as comments/headers and skipped — useful so users
// can group their custom scriptures into sections in the markdown file.
func Load(vaultRoot string) []Scripture {
	path := filepath.Join(vaultRoot, ".granit", "scriptures.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return Defaults()
	}

	var scriptures []Scripture
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		s := Scripture{Text: line}
		// LastIndex (not Index): a verse may itself contain a hyphen.
		// We only treat the FINAL separator as the text/source split.
		for _, sep := range []string{" — ", " – ", " - "} {
			if idx := strings.LastIndex(line, sep); idx > 0 {
				s.Text = strings.TrimSpace(line[:idx])
				s.Source = strings.TrimSpace(line[idx+len(sep):])
				break
			}
		}
		if s.Text != "" {
			scriptures = append(scriptures, s)
		}
	}
	if len(scriptures) == 0 {
		return Defaults()
	}
	return scriptures
}

// Daily returns a deterministic verse for today — same input across
// every device on the same day, so a phone and a laptop see the same
// verse. Rotation seed combines day-of-year and year so the cycle
// shifts across years even with the same vault.
func Daily(vaultRoot string) Scripture {
	all := Load(vaultRoot)
	if len(all) == 0 {
		return Defaults()[0]
	}
	now := time.Now()
	idx := now.YearDay() + now.Year()*367
	return all[idx%len(all)]
}

// Random returns one verse uniformly from the loaded set. Used by the
// TUI's "another one" button; the web's quiz mode uses the full set
// directly so it can pick without replacement.
func Random(vaultRoot string) Scripture {
	all := Load(vaultRoot)
	if len(all) == 0 {
		return Defaults()[0]
	}
	return all[rand.Intn(len(all))]
}

// Topics returns the sorted, deduplicated list of theme tags present in
// the current catalogue, with a count of verses per topic. Empty for
// vaults whose .granit/scriptures.md overrides the defaults (since
// user-edited lines don't carry topic metadata yet) — callers can fall
// back to free-text search in that case.
func Topics(vaultRoot string) []TopicCount {
	all := Load(vaultRoot)
	counts := map[string]int{}
	for _, s := range all {
		for _, t := range s.Topics {
			counts[t]++
		}
	}
	out := make([]TopicCount, 0, len(counts))
	for k, v := range counts {
		out = append(out, TopicCount{Topic: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Topic < out[j].Topic
	})
	return out
}

// TopicCount pairs a topic tag with how many verses currently carry it.
// Surfaced by /api/v1/scripture/topics so the topical-browse UI can
// render chip labels with counts without re-walking the catalogue.
type TopicCount struct {
	Topic string `json:"topic"`
	Count int    `json:"count"`
}

// ByTopic returns every verse tagged with the given topic. Case-insensitive
// match on the topic string. Order matches the underlying catalogue order
// so the same topic page renders stably across loads.
func ByTopic(vaultRoot, topic string) []Scripture {
	want := strings.ToLower(strings.TrimSpace(topic))
	if want == "" {
		return nil
	}
	all := Load(vaultRoot)
	out := make([]Scripture, 0, 8)
	for _, s := range all {
		for _, t := range s.Topics {
			if strings.ToLower(t) == want {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// Defaults is the seed scripture set shipped with granit. A broad sweep
// across both testaments — Psalms and Proverbs as the wisdom backbone,
// the major Pauline epistles, the Gospels, plus anchor verses from
// Genesis, Isaiah, and Revelation. Each entry carries `Topics` tags so
// the web's topical-browse mode can group by theme without an external
// concordance.
//
// Translation policy: an eclectic mix favouring readability — most
// entries are NIV-flavoured paraphrases that have been the common form
// in granit since the TUI shipped. Where wording is ambiguous we lean
// toward the WEB (which is also bundled fully under /bible). The user
// can always override the whole catalogue by populating
// .granit/scriptures.md.
func Defaults() []Scripture {
	return []Scripture{
		// ── Old Testament: Pentateuch / Historical ────────────────
		{Text: "In the beginning God created the heavens and the earth.", Source: "Genesis 1:1", Topics: []string{"creation", "wonder"}},
		{Text: "So God created mankind in his own image, in the image of God he created them; male and female he created them.", Source: "Genesis 1:27", Topics: []string{"identity", "creation", "dignity"}},
		{Text: "The LORD bless you and keep you; the LORD make his face shine on you and be gracious to you; the LORD turn his face toward you and give you peace.", Source: "Numbers 6:24-26", Topics: []string{"blessing", "peace"}},
		{Text: "Be strong and courageous. Do not be afraid; do not be discouraged, for the LORD your God will be with you wherever you go.", Source: "Joshua 1:9", Topics: []string{"courage", "fear", "presence"}},
		{Text: "The LORD will fight for you; you need only to be still.", Source: "Exodus 14:14", Topics: []string{"trust", "peace", "deliverance"}},
		{Text: "Hear, O Israel: The LORD our God, the LORD is one. Love the LORD your God with all your heart and with all your soul and with all your strength.", Source: "Deuteronomy 6:4-5", Topics: []string{"love", "devotion"}},

		// ── Psalms — the prayer book ─────────────────────────────
		{Text: "Blessed is the one who does not walk in step with the wicked or stand in the way that sinners take or sit in the company of mockers, but whose delight is in the law of the LORD.", Source: "Psalm 1:1-2", Topics: []string{"wisdom", "righteousness"}},
		{Text: "The LORD is my shepherd, I lack nothing.", Source: "Psalm 23:1", Topics: []string{"trust", "provision", "comfort"}},
		{Text: "Even though I walk through the darkest valley, I will fear no evil, for you are with me; your rod and your staff, they comfort me.", Source: "Psalm 23:4", Topics: []string{"fear", "comfort", "presence", "suffering"}},
		{Text: "The LORD is my light and my salvation — whom shall I fear? The LORD is the stronghold of my life — of whom shall I be afraid?", Source: "Psalm 27:1", Topics: []string{"fear", "courage", "trust"}},
		{Text: "Wait for the LORD; be strong and take heart and wait for the LORD.", Source: "Psalm 27:14", Topics: []string{"patience", "courage", "waiting"}},
		{Text: "Taste and see that the LORD is good; blessed is the one who takes refuge in him.", Source: "Psalm 34:8", Topics: []string{"goodness", "joy"}},
		{Text: "The LORD is close to the brokenhearted and saves those who are crushed in spirit.", Source: "Psalm 34:18", Topics: []string{"comfort", "suffering", "presence"}},
		{Text: "Delight yourself in the LORD, and he will give you the desires of your heart.", Source: "Psalm 37:4", Topics: []string{"joy", "desire"}},
		{Text: "Be still, and know that I am God.", Source: "Psalm 46:10", Topics: []string{"peace", "stillness", "trust"}},
		{Text: "Create in me a pure heart, O God, and renew a steadfast spirit within me.", Source: "Psalm 51:10", Topics: []string{"repentance", "renewal"}},
		{Text: "Cast your cares on the LORD and he will sustain you; he will never let the righteous be shaken.", Source: "Psalm 55:22", Topics: []string{"anxiety", "trust", "rest"}},
		{Text: "From the ends of the earth I call to you, I call as my heart grows faint; lead me to the rock that is higher than I.", Source: "Psalm 61:2", Topics: []string{"prayer", "weariness"}},
		{Text: "Whom have I in heaven but you? And earth has nothing I desire besides you.", Source: "Psalm 73:25", Topics: []string{"devotion", "desire"}},
		{Text: "This is the day that the LORD has made; let us rejoice and be glad in it.", Source: "Psalm 118:24", Topics: []string{"joy", "gratitude"}},
		{Text: "Your word is a lamp for my feet, a light on my path.", Source: "Psalm 119:105", Topics: []string{"scripture", "guidance"}},
		{Text: "I lift up my eyes to the mountains — where does my help come from? My help comes from the LORD, the Maker of heaven and earth.", Source: "Psalm 121:1-2", Topics: []string{"help", "trust"}},
		{Text: "Unless the LORD builds the house, the builders labor in vain.", Source: "Psalm 127:1", Topics: []string{"work", "humility"}},
		{Text: "Search me, God, and know my heart; test me and know my anxious thoughts. See if there is any offensive way in me, and lead me in the way everlasting.", Source: "Psalm 139:23-24", Topics: []string{"repentance", "self-examination"}},

		// ── Proverbs — wisdom literature ─────────────────────────
		{Text: "The fear of the LORD is the beginning of knowledge, but fools despise wisdom and instruction.", Source: "Proverbs 1:7", Topics: []string{"wisdom", "humility"}},
		{Text: "Trust in the LORD with all your heart and lean not on your own understanding; in all your ways submit to him, and he will make your paths straight.", Source: "Proverbs 3:5-6", Topics: []string{"trust", "guidance", "humility"}},
		{Text: "Above all else, guard your heart, for everything you do flows from it.", Source: "Proverbs 4:23", Topics: []string{"heart", "vigilance"}},
		{Text: "The fear of the LORD is the beginning of wisdom, and knowledge of the Holy One is understanding.", Source: "Proverbs 9:10", Topics: []string{"wisdom"}},
		{Text: "Where there is no guidance, a people falls, but in an abundance of counselors there is safety.", Source: "Proverbs 11:14", Topics: []string{"counsel", "humility"}},
		{Text: "Commit to the LORD whatever you do, and he will establish your plans.", Source: "Proverbs 16:3", Topics: []string{"work", "planning", "trust"}},
		{Text: "Pride goes before destruction, a haughty spirit before a fall.", Source: "Proverbs 16:18", Topics: []string{"pride", "humility"}},
		{Text: "A friend loves at all times, and a brother is born for a time of adversity.", Source: "Proverbs 17:17", Topics: []string{"friendship", "love"}},
		{Text: "The plans of the diligent lead to profit as surely as haste leads to poverty.", Source: "Proverbs 21:5", Topics: []string{"work", "discipline", "patience"}},
		{Text: "A good name is more desirable than great riches; to be esteemed is better than silver or gold.", Source: "Proverbs 22:1", Topics: []string{"integrity", "wealth"}},
		{Text: "Iron sharpens iron, and one man sharpens another.", Source: "Proverbs 27:17", Topics: []string{"friendship", "growth"}},
		{Text: "The righteous are as bold as a lion.", Source: "Proverbs 28:1", Topics: []string{"courage", "righteousness"}},

		// ── Ecclesiastes / Wisdom poetry ─────────────────────────
		{Text: "There is a time for everything, and a season for every activity under the heavens.", Source: "Ecclesiastes 3:1", Topics: []string{"time", "wisdom"}},
		{Text: "He has made everything beautiful in its time. He has also set eternity in the human heart.", Source: "Ecclesiastes 3:11", Topics: []string{"time", "wonder", "beauty"}},
		{Text: "Two are better than one, because they have a good return for their labor.", Source: "Ecclesiastes 4:9", Topics: []string{"friendship", "work"}},
		{Text: "Whatever your hand finds to do, do it with all your might.", Source: "Ecclesiastes 9:10", Topics: []string{"work", "diligence"}},

		// ── Prophets ─────────────────────────────────────────────
		{Text: "Come now, let us settle the matter. Though your sins are like scarlet, they shall be as white as snow.", Source: "Isaiah 1:18", Topics: []string{"repentance", "forgiveness"}},
		{Text: "You will keep in perfect peace those whose minds are steadfast, because they trust in you.", Source: "Isaiah 26:3", Topics: []string{"peace", "trust", "anxiety"}},
		{Text: "But those who hope in the LORD will renew their strength. They will soar on wings like eagles; they will run and not grow weary, they will walk and not be faint.", Source: "Isaiah 40:31", Topics: []string{"hope", "strength", "weariness"}},
		{Text: "Do not fear, for I am with you; do not be dismayed, for I am your God. I will strengthen you and help you; I will uphold you with my righteous right hand.", Source: "Isaiah 41:10", Topics: []string{"fear", "strength", "presence"}},
		{Text: "He was pierced for our transgressions, he was crushed for our iniquities; the punishment that brought us peace was on him, and by his wounds we are healed.", Source: "Isaiah 53:5", Topics: []string{"redemption", "suffering", "healing"}},
		{Text: "Seek the LORD while he may be found; call on him while he is near.", Source: "Isaiah 55:6", Topics: []string{"seeking", "repentance"}},
		{Text: "\"For my thoughts are not your thoughts, neither are your ways my ways,\" declares the LORD.", Source: "Isaiah 55:8", Topics: []string{"humility", "wisdom"}},
		{Text: "\"For I know the plans I have for you,\" declares the LORD, \"plans to prosper you and not to harm you, plans to give you hope and a future.\"", Source: "Jeremiah 29:11", Topics: []string{"hope", "future", "trust"}},
		{Text: "\"You will seek me and find me when you seek me with all your heart.\"", Source: "Jeremiah 29:13", Topics: []string{"seeking", "devotion"}},
		{Text: "Because of the LORD's great love we are not consumed, for his compassions never fail. They are new every morning; great is your faithfulness.", Source: "Lamentations 3:22-23", Topics: []string{"mercy", "faithfulness", "renewal"}},
		{Text: "He has shown you, O mortal, what is good. And what does the LORD require of you? To act justly and to love mercy and to walk humbly with your God.", Source: "Micah 6:8", Topics: []string{"justice", "mercy", "humility"}},
		{Text: "The LORD your God is with you, the Mighty Warrior who saves. He will take great delight in you; in his love he will no longer rebuke you, but will rejoice over you with singing.", Source: "Zephaniah 3:17", Topics: []string{"love", "joy", "presence"}},

		// ── Gospels ──────────────────────────────────────────────
		{Text: "Blessed are the poor in spirit, for theirs is the kingdom of heaven.", Source: "Matthew 5:3", Topics: []string{"humility", "blessing"}},
		{Text: "Blessed are those who mourn, for they will be comforted.", Source: "Matthew 5:4", Topics: []string{"grief", "comfort"}},
		{Text: "Blessed are the meek, for they will inherit the earth.", Source: "Matthew 5:5", Topics: []string{"humility"}},
		{Text: "Blessed are those who hunger and thirst for righteousness, for they will be filled.", Source: "Matthew 5:6", Topics: []string{"righteousness", "desire"}},
		{Text: "Blessed are the merciful, for they will be shown mercy.", Source: "Matthew 5:7", Topics: []string{"mercy"}},
		{Text: "Blessed are the pure in heart, for they will see God.", Source: "Matthew 5:8", Topics: []string{"purity"}},
		{Text: "Blessed are the peacemakers, for they will be called children of God.", Source: "Matthew 5:9", Topics: []string{"peace"}},
		{Text: "You are the light of the world. A town built on a hill cannot be hidden.", Source: "Matthew 5:14", Topics: []string{"identity", "witness"}},
		{Text: "Therefore do not worry about tomorrow, for tomorrow will worry about itself. Each day has enough trouble of its own.", Source: "Matthew 6:34", Topics: []string{"anxiety", "presence", "trust"}},
		{Text: "Ask and it will be given to you; seek and you will find; knock and the door will be opened to you.", Source: "Matthew 7:7", Topics: []string{"prayer", "seeking"}},
		{Text: "\"Come to me, all you who are weary and burdened, and I will give you rest.\"", Source: "Matthew 11:28", Topics: []string{"rest", "weariness", "comfort"}},
		{Text: "\"For where your treasure is, there your heart will be also.\"", Source: "Matthew 6:21", Topics: []string{"heart", "wealth"}},
		{Text: "\"But seek first his kingdom and his righteousness, and all these things will be given to you as well.\"", Source: "Matthew 6:33", Topics: []string{"priorities", "trust", "kingdom"}},
		{Text: "\"What good will it be for someone to gain the whole world, yet forfeit their soul?\"", Source: "Matthew 16:26", Topics: []string{"identity", "wealth"}},
		{Text: "\"Love the Lord your God with all your heart and with all your soul and with all your mind.\" This is the first and greatest commandment. And the second is like it: \"Love your neighbor as yourself.\"", Source: "Matthew 22:37-39", Topics: []string{"love", "devotion"}},
		{Text: "\"Whoever wants to become great among you must be your servant.\"", Source: "Mark 10:43", Topics: []string{"service", "humility", "leadership"}},
		{Text: "\"Therefore I tell you, whatever you ask for in prayer, believe that you have received it, and it will be yours.\"", Source: "Mark 11:24", Topics: []string{"prayer", "faith"}},
		{Text: "\"For nothing will be impossible with God.\"", Source: "Luke 1:37", Topics: []string{"faith", "hope"}},
		{Text: "\"My soul magnifies the Lord, and my spirit rejoices in God my Savior.\"", Source: "Luke 1:46-47", Topics: []string{"joy", "worship"}},
		{Text: "\"Forgive, and you will be forgiven. Give, and it will be given to you.\"", Source: "Luke 6:37-38", Topics: []string{"forgiveness", "generosity"}},
		{Text: "He who is faithful in a very little thing is faithful also in much.", Source: "Luke 16:10", Topics: []string{"faithfulness", "diligence"}},
		{Text: "\"For the Son of Man came to seek and to save the lost.\"", Source: "Luke 19:10", Topics: []string{"redemption", "grace"}},
		{Text: "In the beginning was the Word, and the Word was with God, and the Word was God.", Source: "John 1:1", Topics: []string{"identity", "creation"}},
		{Text: "The Word became flesh and made his dwelling among us. We have seen his glory, the glory of the one and only Son.", Source: "John 1:14", Topics: []string{"incarnation", "presence"}},
		{Text: "\"For God so loved the world that he gave his one and only Son, that whoever believes in him shall not perish but have eternal life.\"", Source: "John 3:16", Topics: []string{"love", "salvation", "grace"}},
		{Text: "\"I am the bread of life. Whoever comes to me will never go hungry.\"", Source: "John 6:35", Topics: []string{"identity", "provision"}},
		{Text: "\"Then you will know the truth, and the truth will set you free.\"", Source: "John 8:32", Topics: []string{"truth", "freedom"}},
		{Text: "\"I am the good shepherd. The good shepherd lays down his life for the sheep.\"", Source: "John 10:11", Topics: []string{"identity", "sacrifice"}},
		{Text: "\"I have come that they may have life, and have it to the full.\"", Source: "John 10:10", Topics: []string{"life", "abundance"}},
		{Text: "\"I am the way and the truth and the life. No one comes to the Father except through me.\"", Source: "John 14:6", Topics: []string{"identity", "truth"}},
		{Text: "\"Peace I leave with you; my peace I give you. I do not give to you as the world gives. Do not let your hearts be troubled and do not be afraid.\"", Source: "John 14:27", Topics: []string{"peace", "fear"}},
		{Text: "\"I am the vine; you are the branches. If you remain in me and I in you, you will bear much fruit; apart from me you can do nothing.\"", Source: "John 15:5", Topics: []string{"abiding", "fruit"}},
		{Text: "\"Greater love has no one than this: to lay down one's life for one's friends.\"", Source: "John 15:13", Topics: []string{"love", "sacrifice", "friendship"}},

		// ── Acts ────────────────────────────────────────────────
		{Text: "It is more blessed to give than to receive.", Source: "Acts 20:35", Topics: []string{"generosity"}},

		// ── Pauline epistles ────────────────────────────────────
		{Text: "Therefore, since we have been justified through faith, we have peace with God through our Lord Jesus Christ.", Source: "Romans 5:1", Topics: []string{"peace", "faith", "salvation"}},
		{Text: "But God demonstrates his own love for us in this: While we were still sinners, Christ died for us.", Source: "Romans 5:8", Topics: []string{"love", "grace"}},
		{Text: "And we know that in all things God works for the good of those who love him, who have been called according to his purpose.", Source: "Romans 8:28", Topics: []string{"hope", "providence", "trust"}},
		{Text: "If God is for us, who can be against us?", Source: "Romans 8:31", Topics: []string{"courage", "trust"}},
		{Text: "Neither height nor depth, nor anything else in all creation, will be able to separate us from the love of God that is in Christ Jesus our Lord.", Source: "Romans 8:38-39", Topics: []string{"love", "security"}},
		{Text: "Do not conform to the pattern of this world, but be transformed by the renewing of your mind.", Source: "Romans 12:2", Topics: []string{"renewal", "wisdom"}},
		{Text: "Be devoted to one another in love. Honor one another above yourselves.", Source: "Romans 12:10", Topics: []string{"love", "community"}},
		{Text: "Rejoice with those who rejoice; mourn with those who mourn.", Source: "Romans 12:15", Topics: []string{"empathy", "community"}},
		{Text: "If it is possible, as far as it depends on you, live at peace with everyone.", Source: "Romans 12:18", Topics: []string{"peace", "relationships"}},
		{Text: "Love is patient, love is kind. It does not envy, it does not boast, it is not proud.", Source: "1 Corinthians 13:4", Topics: []string{"love"}},
		{Text: "Love bears all things, believes all things, hopes all things, endures all things.", Source: "1 Corinthians 13:7", Topics: []string{"love", "endurance"}},
		{Text: "Love never fails.", Source: "1 Corinthians 13:8", Topics: []string{"love"}},
		{Text: "And now these three remain: faith, hope and love. But the greatest of these is love.", Source: "1 Corinthians 13:13", Topics: []string{"love", "faith", "hope"}},
		{Text: "Therefore, my dear brothers and sisters, stand firm. Let nothing move you. Always give yourselves fully to the work of the Lord.", Source: "1 Corinthians 15:58", Topics: []string{"perseverance", "work"}},
		{Text: "Do everything in love.", Source: "1 Corinthians 16:14", Topics: []string{"love"}},
		{Text: "Therefore, if anyone is in Christ, the new creation has come: The old has gone, the new is here!", Source: "2 Corinthians 5:17", Topics: []string{"renewal", "identity"}},
		{Text: "We live by faith, not by sight.", Source: "2 Corinthians 5:7", Topics: []string{"faith"}},
		{Text: "\"My grace is sufficient for you, for my power is made perfect in weakness.\"", Source: "2 Corinthians 12:9", Topics: []string{"grace", "weakness", "suffering"}},
		{Text: "But the fruit of the Spirit is love, joy, peace, forbearance, kindness, goodness, faithfulness, gentleness and self-control.", Source: "Galatians 5:22-23", Topics: []string{"character", "fruit"}},
		{Text: "And let us not grow weary of doing good, for in due season we will reap, if we do not give up.", Source: "Galatians 6:9", Topics: []string{"perseverance", "work"}},
		{Text: "For we are God's handiwork, created in Christ Jesus to do good works, which God prepared in advance for us to do.", Source: "Ephesians 2:10", Topics: []string{"identity", "purpose", "work"}},
		{Text: "Be completely humble and gentle; be patient, bearing with one another in love.", Source: "Ephesians 4:2", Topics: []string{"humility", "patience", "love"}},
		{Text: "\"In your anger do not sin\": Do not let the sun go down while you are still angry.", Source: "Ephesians 4:26", Topics: []string{"anger", "reconciliation"}},
		{Text: "Be kind and compassionate to one another, forgiving each other, just as in Christ God forgave you.", Source: "Ephesians 4:32", Topics: []string{"kindness", "forgiveness"}},
		{Text: "Be very careful, then, how you live — not as unwise but as wise, making the most of every opportunity.", Source: "Ephesians 5:15-16", Topics: []string{"wisdom", "time"}},
		{Text: "Finally, be strong in the Lord and in his mighty power.", Source: "Ephesians 6:10", Topics: []string{"strength"}},
		{Text: "Being confident of this, that he who began a good work in you will carry it on to completion until the day of Christ Jesus.", Source: "Philippians 1:6", Topics: []string{"hope", "perseverance"}},
		{Text: "Do nothing out of selfish ambition or vain conceit. Rather, in humility value others above yourselves.", Source: "Philippians 2:3", Topics: []string{"humility", "service"}},
		{Text: "Rejoice in the Lord always. I will say it again: Rejoice!", Source: "Philippians 4:4", Topics: []string{"joy"}},
		{Text: "Do not be anxious about anything, but in every situation, by prayer and petition, with thanksgiving, present your requests to God.", Source: "Philippians 4:6", Topics: []string{"anxiety", "prayer", "gratitude"}},
		{Text: "And the peace of God, which transcends all understanding, will guard your hearts and your minds in Christ Jesus.", Source: "Philippians 4:7", Topics: []string{"peace", "anxiety"}},
		{Text: "Whatever is true, whatever is noble, whatever is right, whatever is pure, whatever is lovely, whatever is admirable — if anything is excellent or praiseworthy — think about such things.", Source: "Philippians 4:8", Topics: []string{"thoughts", "purity"}},
		{Text: "I have learned to be content whatever the circumstances.", Source: "Philippians 4:11", Topics: []string{"contentment", "peace"}},
		{Text: "I can do all things through Christ who strengthens me.", Source: "Philippians 4:13", Topics: []string{"strength", "perseverance"}},
		{Text: "And my God will meet all your needs according to the riches of his glory in Christ Jesus.", Source: "Philippians 4:19", Topics: []string{"provision", "trust"}},
		{Text: "Set your minds on things above, not on earthly things.", Source: "Colossians 3:2", Topics: []string{"priorities", "thoughts"}},
		{Text: "Whatever you do, work at it with all your heart, as working for the Lord, not for human masters.", Source: "Colossians 3:23", Topics: []string{"work", "diligence"}},
		{Text: "Let your conversation be always full of grace, seasoned with salt, so that you may know how to answer everyone.", Source: "Colossians 4:6", Topics: []string{"speech", "grace"}},
		{Text: "Rejoice always, pray continually, give thanks in all circumstances; for this is God's will for you in Christ Jesus.", Source: "1 Thessalonians 5:16-18", Topics: []string{"joy", "prayer", "gratitude"}},
		{Text: "For God gave us a spirit not of fear but of power and love and self-discipline.", Source: "2 Timothy 1:7", Topics: []string{"fear", "courage", "self-control"}},
		{Text: "All Scripture is God-breathed and is useful for teaching, rebuking, correcting and training in righteousness.", Source: "2 Timothy 3:16", Topics: []string{"scripture"}},
		{Text: "I have fought the good fight, I have finished the race, I have kept the faith.", Source: "2 Timothy 4:7", Topics: []string{"perseverance", "faithfulness"}},

		// ── Hebrews / General epistles ──────────────────────────
		{Text: "Now faith is confidence in what we hope for and assurance about what we do not see.", Source: "Hebrews 11:1", Topics: []string{"faith", "hope"}},
		{Text: "Let us run with perseverance the race marked out for us, fixing our eyes on Jesus, the pioneer and perfecter of faith.", Source: "Hebrews 12:1-2", Topics: []string{"perseverance", "focus"}},
		{Text: "No discipline seems pleasant at the time, but painful. Later on, however, it produces a harvest of righteousness and peace for those who have been trained by it.", Source: "Hebrews 12:11", Topics: []string{"discipline", "growth"}},
		{Text: "Keep your lives free from the love of money and be content with what you have, because God has said, \"Never will I leave you; never will I forsake you.\"", Source: "Hebrews 13:5", Topics: []string{"contentment", "presence", "wealth"}},
		{Text: "Consider it pure joy, my brothers and sisters, whenever you face trials of many kinds, because you know that the testing of your faith produces perseverance.", Source: "James 1:2-3", Topics: []string{"joy", "trials", "perseverance"}},
		{Text: "If any of you lacks wisdom, you should ask God, who gives generously to all without finding fault, and it will be given to you.", Source: "James 1:5", Topics: []string{"wisdom", "prayer"}},
		{Text: "Blessed is the one who perseveres under trial because, having stood the test, that person will receive the crown of life.", Source: "James 1:12", Topics: []string{"perseverance", "trials"}},
		{Text: "Everyone should be quick to listen, slow to speak and slow to become angry.", Source: "James 1:19", Topics: []string{"listening", "speech", "anger"}},
		{Text: "Faith by itself, if it is not accompanied by action, is dead.", Source: "James 2:17", Topics: []string{"faith", "action"}},
		{Text: "Submit yourselves, then, to God. Resist the devil, and he will flee from you. Come near to God and he will come near to you.", Source: "James 4:7-8", Topics: []string{"surrender", "presence"}},
		{Text: "Cast all your anxiety on him because he cares for you.", Source: "1 Peter 5:7", Topics: []string{"anxiety", "trust"}},
		{Text: "Above all, love each other deeply, because love covers over a multitude of sins.", Source: "1 Peter 4:8", Topics: []string{"love", "forgiveness"}},
		{Text: "If we confess our sins, he is faithful and just and will forgive us our sins and purify us from all unrighteousness.", Source: "1 John 1:9", Topics: []string{"repentance", "forgiveness"}},
		{Text: "There is no fear in love. But perfect love drives out fear.", Source: "1 John 4:18", Topics: []string{"love", "fear"}},
		{Text: "We love because he first loved us.", Source: "1 John 4:19", Topics: []string{"love"}},

		// ── Revelation ──────────────────────────────────────────
		{Text: "\"He will wipe every tear from their eyes. There will be no more death or mourning or crying or pain, for the old order of things has passed away.\"", Source: "Revelation 21:4", Topics: []string{"hope", "comfort", "renewal"}},
		{Text: "\"I am the Alpha and the Omega, the First and the Last, the Beginning and the End.\"", Source: "Revelation 22:13", Topics: []string{"identity", "eternity"}},
	}
}
