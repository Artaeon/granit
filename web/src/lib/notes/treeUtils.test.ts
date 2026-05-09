import { describe, expect, it } from 'vitest';
import { buildTree, filterTree, ancestorFolders } from './treeUtils';
import type { Note } from '$lib/api';

function note(path: string, modTime: string): Note {
  return {
    path,
    title: path.split('/').pop()!.replace(/\.md$/, ''),
    modTime,
    size: 0
  };
}

describe('buildTree', () => {
  it('builds a flat root for top-level notes', () => {
    const tree = buildTree([
      note('alpha.md', '2026-01-01T00:00:00Z'),
      note('bravo.md', '2026-01-01T00:00:00Z')
    ]);
    expect(tree.children?.map((c) => c.name)).toEqual(['alpha.md', 'bravo.md']);
  });

  it('nests under folders', () => {
    const tree = buildTree([
      note('Daily/2026-01-01.md', '2026-01-01T00:00:00Z'),
      note('Daily/2026-01-02.md', '2026-01-02T00:00:00Z'),
      note('Inbox/idea.md', '2026-01-03T00:00:00Z')
    ]);
    const names = tree.children?.map((c) => c.name);
    expect(names).toEqual(['Daily', 'Inbox']);
    const daily = tree.children?.find((c) => c.name === 'Daily');
    expect(daily?.children?.map((c) => c.name)).toEqual([
      '2026-01-01.md',
      '2026-01-02.md'
    ]);
  });

  it('puts folders before files within the same parent', () => {
    const tree = buildTree([
      note('zzz-leaf.md', '2026-01-01T00:00:00Z'),
      note('AAA/note.md', '2026-01-01T00:00:00Z')
    ]);
    expect(tree.children?.map((c) => c.name)).toEqual(['AAA', 'zzz-leaf.md']);
  });

  it('counts descendants recursively', () => {
    const tree = buildTree([
      note('A/B/x.md', '2026-01-01T00:00:00Z'),
      note('A/B/y.md', '2026-01-01T00:00:00Z'),
      note('A/c.md', '2026-01-01T00:00:00Z')
    ]);
    expect(tree.count).toBe(3);
    const a = tree.children?.[0];
    expect(a?.count).toBe(3);
    const b = a?.children?.find((c) => c.name === 'B');
    expect(b?.count).toBe(2);
  });

  describe('sort=recent', () => {
    it('floats the most-recently-modified note within a folder', () => {
      const tree = buildTree(
        [
          note('zeta.md', '2026-01-03T00:00:00Z'), // newest
          note('alpha.md', '2026-01-01T00:00:00Z'),
          note('bravo.md', '2026-01-02T00:00:00Z')
        ],
        'recent'
      );
      expect(tree.children?.map((c) => c.name)).toEqual([
        'zeta.md',
        'bravo.md',
        'alpha.md'
      ]);
    });

    it('folders sort by their newest descendant', () => {
      // 'Old' folder has a Jan-01 note; 'Recent' has a Jan-10
      // note. 'Recent' should come first under 'recent' sort.
      const tree = buildTree(
        [
          note('Old/note.md', '2026-01-01T00:00:00Z'),
          note('Recent/note.md', '2026-01-10T00:00:00Z')
        ],
        'recent'
      );
      expect(tree.children?.map((c) => c.name)).toEqual(['Recent', 'Old']);
    });

    it('ties break alphabetically so renders are stable', () => {
      const tree = buildTree(
        [
          note('bravo.md', '2026-01-01T00:00:00Z'),
          note('alpha.md', '2026-01-01T00:00:00Z')
        ],
        'recent'
      );
      expect(tree.children?.map((c) => c.name)).toEqual(['alpha.md', 'bravo.md']);
    });

    it('folders still come before files even under recent sort', () => {
      const tree = buildTree(
        [
          note('zzz.md', '2026-12-01T00:00:00Z'), // very recent file
          note('Folder/old.md', '2026-01-01T00:00:00Z') // ancient folder
        ],
        'recent'
      );
      // Folder first regardless of its older newest-descendant.
      expect(tree.children?.map((c) => c.name)).toEqual(['Folder', 'zzz.md']);
    });
  });
});

describe('filterTree', () => {
  it('returns the whole tree on empty query', () => {
    const tree = buildTree([note('a.md', '2026-01-01T00:00:00Z')]);
    expect(filterTree(tree, '')).toBe(tree);
  });

  it('keeps folders that contain matches and drops empty branches', () => {
    const tree = buildTree([
      note('Hits/match-me.md', '2026-01-01T00:00:00Z'),
      note('Misses/something.md', '2026-01-01T00:00:00Z')
    ]);
    const filtered = filterTree(tree, 'match');
    expect(filtered).not.toBeNull();
    const folderNames = filtered!.children?.map((c) => c.name);
    expect(folderNames).toEqual(['Hits']);
  });

  it('returns null when nothing matches', () => {
    const tree = buildTree([note('a.md', '2026-01-01T00:00:00Z')]);
    expect(filterTree(tree, 'nope')).toBeNull();
  });

  it('matches on path or filename', () => {
    const tree = buildTree([note('SubFolder/specific.md', '2026-01-01T00:00:00Z')]);
    expect(filterTree(tree, 'subfolder')).not.toBeNull();
    expect(filterTree(tree, 'specific')).not.toBeNull();
  });
});

describe('ancestorFolders', () => {
  it('returns all parent folder paths', () => {
    const got = ancestorFolders('A/B/C/x.md');
    expect([...got].sort()).toEqual(['A', 'A/B', 'A/B/C']);
  });

  it('returns empty for a root file', () => {
    expect(ancestorFolders('top.md').size).toBe(0);
  });
});
