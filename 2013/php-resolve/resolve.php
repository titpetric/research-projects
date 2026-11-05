<?php

class Promise
{
	var $value = array();
	var $resolved = false;

	function getId()
	{
		return false;
	}

	function get()
	{
		if (!$this->resolved) {
			throw new Exception("Promise ".get_called_class()." is not resolved!");
		}
		return $this->value;
	}

	function resolved()
	{
		return $this->resolved;
	}

	function resolve($value)
	{
		$this->value = $value;
		$this->resolved = true;
	}

	function resolveAll() {}
	function getPromises() { return array(); }
}

class Main extends Promise
{
	function getPromises()
	{
		$section_ids = array(1,2,3,4);
		$sections = array();
		$increment = 1;
		foreach ($section_ids as $section_id) {
			$section = new SectionItems($section_id, 4, $increment);
			$promises = $section->getPromises();
			$sections[$section_id] = array("value" => $section, "promises" => $promises);
			$increment ++;
		}
		return array("sections"=>$sections);
	}
}

class SectionItems extends Promise
{
	public static $news_ids = array();

	public $section_id;
	public $length;
	public $increment;

	function __construct($section_id, $length, $increment)
	{
		//echo "section::promise $section_id\n";
		$this->section_id = $section_id;
		$this->length = $length;
		$this->increment = $increment;
	}

	function getId()
	{
		return $this->section_id;
	}

	function getPromises()
	{
		$id = 1;
		$news_ids = array();
		// select news_id from news.sections where
		// section_id=$section_id and id not in self::$news_ids
		// order by stamp desc limit 0,$length
		while (count($news_ids) < $this->length) {
			while (isset(self::$news_ids[$id])) {
				$id += $this->increment;
			}
			$news_ids[] = $id;
			self::$news_ids[$id] = $id;
			$id += $this->increment;
		}

		$promises = array();
		foreach ($news_ids as $id) {
			$newsitem = new NewsItem($id);
			$promises[$id] = array("value" => $newsitem, "promises" => $newsitem->getPromises());
		}
		return array("items" => $promises);
	}

	function get()
	{
		if (!$this->resolved()) {
			$this->resolve(array("id" => $this->getId(), "title" => "Section #" . $this->getId()));
		}
		return $this->value;
	}
}

class NewsItem extends Promise
{
	public $news_id = false;

	function getId()
	{
		return $this->news_id;
	}

	function __construct($news_id)
	{
		//echo "newsitem::promise $news_id\n";
		$this->news_id = $news_id;
	}

	function getPromises()
	{
		return array("comment_count"=>new CommentCount($this->getId()));
	}

	function resolveAll(&$promises)
	{
		//echo __CLASS__."::resolveAll\n";
		$keys = array();
		$news_ids = array();
		foreach ($promises as $key => $promise) {
			$keys[] = $key;
			$news_id = $promise->getId();
			$news_ids[$news_id] = $news_id;
		}

		// select group bla bla, mget, bla bla

		$results = $news_ids;
		foreach ($results as $id => $value) {
			$results[$id] = array("id" => $id, "title" => "Test title (#$id)");
		}

		// map back to promises;
		foreach ($keys as $key) {
			$promises[$key]->resolve($results[$promises[$key]->getId()]);
		}
	}
}

class CommentCount extends Promise
{
	public $news_id = false;

	function __construct($news_id)
	{
		//echo "comment::promise $news_id\n";
		$this->news_id = $news_id;
	}

	function getId()
	{
		return $this->news_id;
	}

	function resolveAll(&$promises)
	{
		//echo __CLASS__."::resolveAll\n";
		$keys = array();
		$news_ids = array();
		foreach ($promises as $key => $promise) {
			$keys[] = $key;
			$news_id = $promise->getId();
			$news_ids[$news_id] = $news_id;
		}

		// select group bla bla, mget, bla bla

		$results = $news_ids;
		foreach ($results as $id => $value) {
			$results[$id] = 100+rand(0,100);
		}

		// map back to promises;
		foreach ($keys as $key) {
			$promises[$key]->resolve($results[$promises[$key]->getId()]);
		}
	}
}

class util
{
	function flat_ref_by_class(&$tree, &$classes)
	{
		foreach ($tree as $key => $value) {
			if (is_object($value)) {
				$classes[get_class($value)][] = $value;
				continue;
			}
			if (is_array($value)) {
				$subclasses = util::flat_ref_by_class($tree[$key], $classes);
			}
		}
		return $classes;
	}

	function resolve_to_array(&$tree)
	{
		$value = array();
		foreach ($tree as $key => $value) {
			if (is_object($value)) {
				$tree[$key] = $value->get();
				continue;
			}
			if (is_array($value)) {
				util::resolve_to_array($tree[$key]);
			}
		}
		if (isset($tree['value'], $tree['promises'])) {
			foreach ($tree['promises'] as $key => $promise) {
				$tree['value'][$key] = $promise;
			}
			$tree = $tree['value'];
		}
	}

	function summary($times)
	{
		$summary = array("count" => count($times), "min"=>min($times), "max"=>max($times), "sum"=>array_sum($times));
		$summary['avg'] = $summary['sum']/$summary['count'];
		sort($times);
		$summary['95th'] = $times[floor($summary['count'] * 0.95)];
		$summary['99th'] = $times[floor($summary['count'] * 0.99)];
		print_r($summary);
	}
}

function resolve_promises($promises)
{
	$flat = array();
	util::flat_ref_by_class($promises, $flat);
	foreach ($flat as $classname => $class_promises) {
		$promise = current($class_promises);
		$promise->resolveAll($flat[$classname]);
	}
	util::resolve_to_array($promises);
	return $promises;
}

$duration = array();
for ($i=0; $i<1000; $i++) {
	$start = microtime(true);
	SectionItems::$news_ids = array();
	$implicit = resolve_promises( Main::getPromises() );
	$end = microtime(true);
	$duration[] = $end-$start;
}

echo json_encode($implicit);

echo "\n";

util::summary($duration);
