// A small built-in rotation. The user can also paste their own in the wizard.
export interface Scripture {
  text: string;
  source: string;
}

export const scriptures: Scripture[] = [
  { text: 'Iron sharpens iron, and one man sharpens another.', source: 'Proverbs 27:17' },
  { text: 'Commit your work to the LORD, and your plans will be established.', source: 'Proverbs 16:3' },
  { text: 'The night is coming when no one can work.', source: 'John 9:4' },
  { text: 'Whatever you do, work heartily, as for the Lord and not for men.', source: 'Colossians 3:23' },
  { text: 'Be strong and courageous. Do not be afraid; do not be discouraged.', source: 'Joshua 1:9' },
  { text: 'In all your ways acknowledge him, and he will make straight your paths.', source: 'Proverbs 3:6' },
  { text: 'I can do all things through him who strengthens me.', source: 'Philippians 4:13' },
  { text: 'A good name is to be chosen rather than great riches.', source: 'Proverbs 22:1' },
  { text: 'Trust in the LORD with all your heart, and do not lean on your own understanding.', source: 'Proverbs 3:5' },
  { text: 'Plans fail for lack of counsel, but with many advisers they succeed.', source: 'Proverbs 15:22' },
  { text: 'Whoever sows sparingly will also reap sparingly.', source: '2 Corinthians 9:6' },
  { text: 'The plans of the diligent lead surely to abundance.', source: 'Proverbs 21:5' },
  { text: 'Do not be anxious about anything.', source: 'Philippians 4:6' },
  { text: 'Let your light shine before others.', source: 'Matthew 5:16' },
  { text: 'Be still, and know that I am God.', source: 'Psalm 46:10' }
];

// Deterministic-per-day scripture: same one all day, rotates by date.
export function scriptureOfTheDay(now = new Date()): Scripture {
  const d = Math.floor(
    Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()) / 86_400_000
  );
  return scriptures[Math.abs(d) % scriptures.length];
}
