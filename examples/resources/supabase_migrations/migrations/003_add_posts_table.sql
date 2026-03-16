-- Add posts table
-- Enables user-generated content

CREATE TABLE IF NOT EXISTS public.posts (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id UUID REFERENCES public.users(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  published BOOLEAN DEFAULT false,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes
CREATE INDEX IF NOT EXISTS posts_user_id_idx ON public.posts(user_id);
CREATE INDEX IF NOT EXISTS posts_published_idx ON public.posts(published);
CREATE INDEX IF NOT EXISTS posts_created_at_idx ON public.posts(created_at DESC);

-- Enable Row Level Security
ALTER TABLE public.posts ENABLE ROW LEVEL SECURITY;

-- RLS Policy: Anyone can read published posts
CREATE POLICY "Published posts are viewable by everyone"
  ON public.posts
  FOR SELECT
  USING (published = true);

-- RLS Policy: Users can read their own unpublished posts
CREATE POLICY "Users can view own unpublished posts"
  ON public.posts
  FOR SELECT
  USING (auth.uid() = user_id);

-- RLS Policy: Users can insert their own posts
CREATE POLICY "Users can insert own posts"
  ON public.posts
  FOR INSERT
  WITH CHECK (auth.uid() = user_id);

-- RLS Policy: Users can update their own posts
CREATE POLICY "Users can update own posts"
  ON public.posts
  FOR UPDATE
  USING (auth.uid() = user_id);

-- RLS Policy: Users can delete their own posts
CREATE POLICY "Users can delete own posts"
  ON public.posts
  FOR DELETE
  USING (auth.uid() = user_id);
